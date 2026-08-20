package repl

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/efigence/rodrev/plugin/puppet"
)

type metaCmd struct {
	Name    string
	Aliases []string
	Args    string
	Help    string
	Run     func(s *Session, args []string) error
}

// metaCommands is a function, not a var, so :help can be built from it without
// an initialization cycle
func metaCommands() []metaCmd {
	return []metaCmd{
		{Name: "help", Aliases: []string{"?"}, Help: "this list", Run: (*Session).metaHelp},
		{Name: "syntax", Help: "query language reference with examples", Run: (*Session).metaSyntax},
		{Name: "fact", Args: "[path|glob]", Help: "show fact value, or list fact names", Run: (*Session).metaFact},
		{Name: "class", Args: "[glob]", Help: "list classes", Run: (*Session).metaClass},
		{Name: "nodes", Args: "[-r]", Help: "list known nodes, -r to refresh", Run: (*Session).metaNodes},
		{Name: "snapshot", Args: "[fqdn]", Help: "load a node's facts/classes for completion and :fact", Run: (*Session).metaSnapshot},
		{Name: "local", Args: "[facts [classes]]", Help: "evaluate locally: against files, or against the loaded snapshot", Run: (*Session).metaLocal},
		{Name: "cluster", Help: "evaluate on the cluster", Run: (*Session).metaCluster},
		{Name: "out", Args: "stderr|csv|json", Help: "output format for results", Run: (*Session).metaOut},
		{Name: "timeout", Args: "<duration>", Help: "how long to wait for results", Run: (*Session).metaTimeout},
		{Name: "verbose", Help: "toggle printing nodes that did not match", Run: (*Session).metaVerbose},
		{Name: "quit", Aliases: []string{"q", "exit"}, Help: "exit (same as Ctrl-D)", Run: (*Session).metaQuit},
	}
}

// metaNames returns all command names, for completion
func metaNames() []string {
	cmds := metaCommands()
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

// lookupMeta resolves a name, an alias, or an unambiguous prefix of either
func lookupMeta(name string) (metaCmd, []string, bool) {
	var matches []metaCmd
	var names []string
	for _, c := range metaCommands() {
		if c.Name == name {
			return c, nil, true
		}
		for _, a := range c.Aliases {
			if a == name {
				return c, nil, true
			}
		}
	}
	for _, c := range metaCommands() {
		if strings.HasPrefix(c.Name, name) {
			matches = append(matches, c)
			names = append(names, ":"+c.Name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil, true
	}
	return metaCmd{}, names, false
}

func (s *Session) runMeta(line string) error {
	fields := strings.Fields(strings.TrimPrefix(line, ":"))
	if len(fields) == 0 {
		s.printf("%s", Help())
		return nil
	}
	cmd, similar, ok := lookupMeta(fields[0])
	if !ok {
		if len(similar) > 1 {
			s.errorf("ambiguous command :%s, matches %s", fields[0], strings.Join(similar, " "))
		} else {
			s.errorf("unknown command :%s, try :help", fields[0])
		}
		return nil
	}
	if err := cmd.Run(s, fields[1:]); err != nil {
		s.errorf("%s", err)
	}
	return nil
}

func (s *Session) metaHelp(args []string) error {
	fmt.Fprint(s.out, Help())
	return nil
}

func (s *Session) metaSyntax(args []string) error {
	fmt.Fprintln(s.out, Syntax())
	return nil
}

func (s *Session) metaQuit(args []string) error {
	s.quit = true
	return nil
}

func (s *Session) metaVerbose(args []string) error {
	s.verbose = !s.verbose
	s.printf("verbose: %v", s.verbose)
	return nil
}

func (s *Session) metaOut(args []string) error {
	if len(args) == 0 {
		s.printf("output format: %s", s.format)
		return nil
	}
	switch args[0] {
	case FormatHuman, FormatCSV, FormatJSON:
		s.format = args[0]
		s.csvHeader = make(map[string]bool, 2)
		s.printf("output format: %s", s.format)
		return nil
	default:
		return fmt.Errorf("unknown format %s, use one of: %s %s %s", args[0], FormatHuman, FormatCSV, FormatJSON)
	}
}

func (s *Session) metaTimeout(args []string) error {
	if len(args) == 0 {
		s.printf("timeout: %s", s.timeout)
		return nil
	}
	d, err := time.ParseDuration(args[0])
	if err != nil {
		return fmt.Errorf("can't parse duration [%s]: %s", args[0], err)
	}
	if d <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	s.timeout = d
	s.printf("timeout: %s", s.timeout)
	return nil
}

func (s *Session) metaNodes(args []string) error {
	refresh := false
	for _, a := range args {
		if a == "-r" || a == "--refresh" {
			refresh = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	nodes, err := s.backend.Nodes(ctx, refresh)
	if err != nil {
		return err
	}
	s.nodes = nodes
	if len(nodes) == 0 {
		s.printf("no nodes known yet")
		return nil
	}
	sorted := append([]string{}, nodes...)
	sort.Strings(sorted)
	for _, n := range sorted {
		s.printf("  %s", n)
	}
	s.printf("%d nodes", len(sorted))
	if reporter, ok := s.backend.(capabilityReporter); ok {
		if caps := reporter.Capabilities(); len(caps) > 0 {
			s.printf("%s", caps)
		}
	}
	return nil
}

func (s *Session) metaSnapshot(args []string) error {
	fqdn := ""
	if len(args) > 0 {
		fqdn = args[0]
	}
	if len(fqdn) == 0 && s.snapshot != nil {
		s.printf("loaded: %s from %s (%s)", s.snapshot.FQDN, s.snapshot.Source, s.dataSummary())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	snap, err := s.backend.Snapshot(ctx, fqdn)
	if err != nil {
		return err
	}
	s.snapshot = snap
	s.printf("loaded %s from %s: %s", snap.FQDN, snap.Source, s.dataSummary())
	return nil
}

func (s *Session) metaLocal(args []string) error {
	var h *puppet.QueryHarness
	var err error
	switch {
	case len(args) > 0:
		classes := ""
		if len(args) > 1 {
			classes = args[1]
		}
		snap, err := puppet.LoadSnapshotFiles(args[0], classes, "")
		if err != nil {
			return err
		}
		s.snapshot = snap
		h, err = snap.Harness(nil)
		if err != nil {
			return err
		}
	case s.snapshot != nil:
		h, err = s.snapshot.Harness(nil)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("no data loaded: pass fact/class files, or run :snapshot <fqdn> first")
	}
	s.setBackend(NewLocalBackend(h))
	s.printf("evaluating locally: %s (%s)", h.Source, s.dataSummary())
	return nil
}

func (s *Session) metaCluster(args []string) error {
	if s.newCluster == nil {
		return fmt.Errorf("no cluster connection in this session (started in local mode)")
	}
	b, err := s.newCluster()
	if err != nil {
		return err
	}
	s.setBackend(b)
	s.printf("evaluating on the cluster: %s", b.Describe())
	return nil
}

func (s *Session) metaFact(args []string) error {
	facts, ok := s.factData()
	if !ok {
		return nil
	}
	if len(args) == 0 {
		s.noteDataSource()
		names := factChildren(facts, nil)
		s.printf("%s", strings.Join(names, " "))
		s.printf("%d facts. :fact <name> shows a value, :fact <glob> filters", len(names))
		return nil
	}
	arg := args[0]
	s.noteDataSource()
	// a dotted path is a lookup, anything else filters the fact names
	if v, ok := factPath(facts, strings.Split(arg, ".")); ok {
		return s.printFacts(map[string]interface{}{arg: v})
	}
	names := matchNames(factChildren(facts, nil), arg)
	if len(names) == 0 {
		return fmt.Errorf("no fact matches [%s]", arg)
	}
	out := make(map[string]interface{}, len(names))
	for _, n := range names {
		out[n], _ = factPath(facts, []string{n})
	}
	return s.printFacts(out)
}

func (s *Session) metaClass(args []string) error {
	if s.snapshot == nil {
		s.hintNoData()
		return nil
	}
	s.noteDataSource()
	classes := s.snapshot.Classes
	if len(args) > 0 {
		classes = matchNames(classes, args[0])
		if len(classes) == 0 {
			return fmt.Errorf("no class matches [%s]", args[0])
		}
	}
	return s.printClasses(classes)
}

// noteDataSource says where inspected data comes from when it is not the same
// place queries are evaluated - in cluster mode the loaded snapshot is one
// node's data, while a query runs on all of them
func (s *Session) noteDataSource() {
	if s.snapshot == nil || s.format != FormatHuman {
		return
	}
	if _, local := s.backend.(*LocalBackend); local {
		return
	}
	s.printf("(%s, from %s - queries still run on the cluster)", s.snapshot.FQDN, s.snapshot.Source)
}

// factData returns the loaded fact map, printing a hint when there is none
func (s *Session) factData() (map[string]interface{}, bool) {
	if s.snapshot == nil || len(s.snapshot.Facts) == 0 {
		s.hintNoData()
		return nil, false
	}
	return s.snapshot.Facts, true
}

// hintNoData explains how to get fact/class data, once per session
func (s *Session) hintNoData() {
	if s.hintedNoData {
		s.printf("no fact data loaded")
		return
	}
	s.hintedNoData = true
	s.printf("no fact data loaded - run :snapshot <fqdn>, or start with --facts/--data-dir")
}
