package repl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/efigence/rodrev/query"
	"go.uber.org/zap"
)

// output formats, same values the --output-format flag uses
const (
	FormatHuman = "stderr"
	FormatCSV   = "csv"
	FormatJSON  = "json"
)

// DefaultTimeout matches how long PuppetStatus has always waited for replies,
// so a query does not report fewer nodes than `rv puppet --filter ... status`
const DefaultTimeout = time.Second * 4

// streamLimit is how many matching nodes are listed as they arrive. A query on a
// few hundred nodes should not bury the summary line
const streamLimit = 20

type Config struct {
	// Backend evaluates the queries. Required
	Backend Backend
	// Out is where results go, defaults to os.Stdout
	Out io.Writer
	Log *zap.SugaredLogger
	// Format is one of FormatHuman/FormatCSV/FormatJSON
	Format string
	// Timeout is how long to wait for query results
	Timeout time.Duration
	// Snapshot is fact/class data for completion and :fact/:class, when the
	// caller already has it (local mode always does)
	Snapshot *puppet.Snapshot
	// NodeMeta overrides entries of the `node` variable for local evaluation.
	// Whatever is not given is generated out of the facts
	NodeMeta map[string]interface{}
	// NewCluster switches to cluster mode on :cluster. Nil disables the command
	NewCluster func() (Backend, error)
	// Interactive enables the banner, prompt and SIGINT handling
	Interactive bool
	Verbose     bool
	Quiet       bool
}

// Session is a REPL session: it turns lines of input into queries or meta
// commands and renders the results. It has no terminal and no MQ of its own,
// so it can be driven from a test or from a pipe
type Session struct {
	backend     Backend
	newCluster  func() (Backend, error)
	out         io.Writer
	log         *zap.SugaredLogger
	format      string
	timeout     time.Duration
	snapshot    *puppet.Snapshot
	nodeMeta    map[string]interface{}
	nodes       []string
	interactive bool
	verbose     bool
	quiet       bool
	quit        bool
	// stream prints node results as they arrive. Pointless for a backend that
	// only ever has one node to report on
	stream bool
	// streamed/suppressed count what the current query printed, to keep a
	// fleet-wide match from scrolling the summary off the screen
	streamed   int
	suppressed int
	// hintedInterrupt makes the "Ctrl-C on an empty line exits" hint show up
	// once per session
	hintedInterrupt bool
	// hintedNoData makes the "no fact data loaded" hint show up once per session
	hintedNoData bool
	csvHeader    map[string]bool
	// queries/matched/failed are aggregated for the process exit code
	queries int
	matched int
	failed  int
}

func New(cfg Config) (*Session, error) {
	if cfg.Backend == nil {
		return nil, fmt.Errorf("need a backend")
	}
	s := Session{
		newCluster:  cfg.NewCluster,
		out:         cfg.Out,
		log:         cfg.Log,
		format:      cfg.Format,
		timeout:     cfg.Timeout,
		snapshot:    cfg.Snapshot,
		nodeMeta:    cfg.NodeMeta,
		interactive: cfg.Interactive,
		verbose:     cfg.Verbose,
		quiet:       cfg.Quiet,
		csvHeader:   make(map[string]bool, 2),
	}
	s.setBackend(cfg.Backend)
	if s.out == nil {
		s.out = os.Stdout
	}
	if s.log == nil {
		s.log = zap.NewNop().Sugar()
	}
	if len(s.format) == 0 {
		s.format = FormatHuman
	}
	if s.timeout == 0 {
		s.timeout = DefaultTimeout
	}
	// non-blocking: a cluster backend returns whatever its discovery cache holds
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()
	if nodes, err := s.backend.Nodes(ctx, false); err == nil {
		s.nodes = nodes
	}
	return &s, nil
}

// Run reads lines until EOF or :quit
func (s *Session) Run(lr LineReader) error {
	if s.interactive && !s.quiet {
		fmt.Fprint(s.out, s.Banner())
	}
	for !s.quit {
		line, err := lr.Prompt(s.Prompt())
		switch err {
		case nil:
		case io.EOF:
			if s.interactive {
				fmt.Fprintln(s.out)
			}
			return nil
		case ErrInterrupted:
			// the line was thrown away; a Ctrl-C with nothing to throw away
			// comes back as io.EOF and ends the session
			if !s.hintedInterrupt {
				s.hintedInterrupt = true
				s.printf("(line cleared - Ctrl-C on an empty line exits)")
			}
			continue
		default:
			return err
		}
		if err := s.EvalLine(line); err != nil {
			return err
		}
	}
	return nil
}

// RunScript reads lines from r without any terminal handling
func (s *Session) RunScript(r io.Reader) error {
	return s.Run(NewScriptReader(r))
}

// EvalLine handles a single line of input: a meta command, a query, a comment or
// an empty line. Errors in the input are reported to the user, not returned;
// a returned error means the session can not continue
func (s *Session) EvalLine(line string) error {
	line = strings.TrimSpace(line)
	if len(line) == 0 || strings.HasPrefix(line, "#") {
		return nil
	}
	if strings.HasPrefix(line, ":") {
		return s.runMeta(line)
	}
	return s.runQuery(line)
}

func (s *Session) runQuery(expr string) error {
	s.queries++
	if err := query.CheckSyntax(expr); err != nil {
		s.failed++
		s.errorf("%s", err)
		return nil
	}
	ctx, cancel := s.evalContext()
	defer cancel()
	s.streamed, s.suppressed = 0, 0
	results := make([]NodeResult, 0, len(s.nodes)+1)
	sink := func(r NodeResult) {
		results = append(results, r)
		s.streamResult(r)
	}
	sum, err := s.backend.Eval(ctx, expr, sink)
	if err != nil {
		s.failed++
		s.errorf("%s%s", err, connectionHint(err))
		if sum.Responded == 0 {
			return nil
		}
		// a query that failed part way through still has results worth showing
	}
	if sum.Matched > 0 {
		s.matched++
	}
	if sum.Errors > 0 {
		s.failed++
	}
	// with a backend where non-matching nodes stay silent, no answer at all is a
	// legitimate "nothing matched", not a failure
	if sum.Responded == 0 && !sum.Silent {
		s.failed++
	}
	return s.printQuery(sum, results)
}

// connectionHint explains what to do about a lost MQ connection: the client
// reconnects on its own, but subscriptions made while it is down just fail
func connectionHint(err error) string {
	msg := err.Error()
	for _, marker := range []string{"not currently connected", "subscription failed", "connection to"} {
		if strings.Contains(msg, marker) {
			return "\n       the MQ client reconnects in the background, retry in a moment"
		}
	}
	return ""
}

// evalContext bounds a query by the session timeout, and by Ctrl-C when
// interactive - liner has no way to interrupt a prompt, so the signal is only
// wired up while a query is running
func (s *Session) evalContext() (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	if !s.interactive {
		return ctx, cancel
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(sigCh)
		close(sigCh)
		cancel()
	}
}

// Prompt returns the prompt string
func (s *Session) Prompt() string {
	return "rv(" + s.mode() + ")> "
}

// mode is the short name of the current backend, for the prompt
func (s *Session) mode() string {
	desc := s.backend.Describe()
	if idx := strings.IndexByte(desc, ' '); idx > 0 {
		return desc[:idx]
	}
	return desc
}

// ExitCode is the process exit code for non-interactive use: 0 when anything
// matched (or when no query was run at all), 1 when nothing matched, 2 when a
// query failed
func (s *Session) ExitCode() int {
	switch {
	case s.failed > 0:
		return 2
	case s.queries == 0, s.matched > 0:
		return 0
	default:
		return 1
	}
}

// Quit reports whether the session asked to exit
func (s *Session) Quit() bool { return s.quit }

func (s *Session) Close() error { return s.backend.Close() }

// setBackend switches the backend and adjusts how results are printed
func (s *Session) setBackend(b Backend) {
	old := s.backend
	s.backend = b
	_, local := b.(*LocalBackend)
	s.stream = !local
	if old != nil && old != b {
		_ = old.Close()
	}
}

func (s *Session) printf(format string, args ...interface{}) {
	fmt.Fprintf(s.out, format+"\n", args...)
}

func (s *Session) errorf(format string, args ...interface{}) {
	// zygo errors come with a trailing newline of their own
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintf(s.out, "error: %s\n", msg)
}

// queryFuncs returns the function names available in a query, as `(name`, so
// completion inserts the opening paren together with the name
func (s *Session) queryFuncs() []string {
	names := query.Completions()
	if b, ok := s.backend.(*LocalBackend); ok {
		names = append(names, b.Harness().Engine.DataMaps()...)
	} else {
		names = append(names, "fact", "class")
	}
	out := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ParseNodeMeta turns key=value arguments into node metadata
func ParseNodeMeta(args []string) (map[string]interface{}, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make(map[string]interface{}, len(args))
	for _, arg := range args {
		key, value, found := strings.Cut(arg, "=")
		key = strings.TrimSpace(key)
		if !found || len(key) == 0 {
			return nil, fmt.Errorf("node metadata has to be key=value, got [%s]", arg)
		}
		out[key] = value
	}
	return out, nil
}

// harnessMeta is the node metadata to build a local harness with. Nil lets the
// harness generate it out of the facts
func (s *Session) harnessMeta(snap *puppet.Snapshot) map[string]interface{} {
	if len(s.nodeMeta) == 0 {
		return nil
	}
	// start from what the facts say, then apply the overrides
	base := make(map[string]interface{}, len(s.nodeMeta)+4)
	if snap != nil {
		if h, err := snap.Harness(nil); err == nil {
			for k, v := range h.Runtime.Cfg.NodeMeta {
				base[k] = v
			}
		}
	}
	for k, v := range s.nodeMeta {
		base[k] = v
	}
	return base
}

// SetInteractive enables the banner, prompt and Ctrl-C handling. Set it before
// Run() when the line reader decides whether input is a terminal
func (s *Session) SetInteractive(interactive bool) { s.interactive = interactive }
