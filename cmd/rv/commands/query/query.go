package query

import (
	"fmt"
	"os"
	"strings"

	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/efigence/rodrev/query/repl"
	"github.com/efigence/rodrev/util"
	"github.com/spf13/cobra"
)

// Query runs the interactive query REPL, or evaluates expressions passed on the
// command line and exits
func Query(cmd *cobra.Command, args []string) {
	c := cmd.Flags()
	factsPath := util.StringOrPanic(c.GetString("facts"))
	classesPath := util.StringOrPanic(c.GetString("classes"))
	lastRunPath := util.StringOrPanic(c.GetString("last-run-summary"))
	dataDir := util.StringOrPanic(c.GetString("data-dir"))
	localFlag := util.BoolOrPanic(c.GetBool("local"))
	debug := util.BoolOrPanic(c.GetBool("debug"))
	quiet := util.BoolOrPanic(c.GetBool("quiet"))
	format := util.StringOrPanic(c.GetString("output-format"))
	timeout := util.DurationOrPanic(c.GetDuration("timeout"))
	historyFile := util.StringOrPanic(c.GetString("history"))
	noHistory := util.BoolOrPanic(c.GetBool("no-history"))
	exprs, err := c.GetStringArray("eval")
	if err != nil {
		exprs = []string{}
	}
	// a query can also be passed as a positional argument
	if len(args) > 0 {
		exprs = append(exprs, strings.Join(args, " "))
	}
	if len(dataDir) > 0 && (len(factsPath) > 0 || len(classesPath) > 0) {
		fmt.Fprintln(os.Stderr, "error: --data-dir can not be combined with --facts/--classes")
		os.Exit(2)
	}
	local := localFlag || len(factsPath) > 0 || len(classesPath) > 0 || len(dataDir) > 0

	var cfg repl.Config
	cfg.Format = format
	cfg.Timeout = timeout
	cfg.Quiet = quiet
	cfg.Out = os.Stdout

	if local {
		_, log := clinit.InitOffline(cmd)
		cfg.Log = log
		snap, err := loadLocal(dataDir, factsPath, classesPath, lastRunPath)
		if err != nil {
			log.Errorf("%s", err)
			os.Exit(2)
		}
		harness, err := snap.Harness(nil)
		if err != nil {
			log.Errorf("%s", err)
			os.Exit(2)
		}
		cfg.Snapshot = snap
		cfg.Backend = repl.NewLocalBackend(harness)
	} else {
		_, runtime, log := clinit.Init(cmd)
		// the REPL prints its own results, per-request chatter would fight with
		// the prompt
		if !debug {
			runtime.Log = clinit.InitLog(false, true)
		}
		cfg.Log = log
		backend, err := repl.NewClusterBackend(&runtime, log)
		if err != nil {
			log.Errorf("%s", err)
			os.Exit(2)
		}
		cfg.Backend = backend
		if len(factsPath) > 0 || len(dataDir) > 0 {
			// fact/class data for completion and :fact, evaluation still remote
			if snap, err := loadLocal(dataDir, factsPath, classesPath, lastRunPath); err == nil {
				cfg.Snapshot = snap
			} else {
				log.Errorf("%s", err)
			}
		}
	}

	// one-shot mode: evaluate what was asked for and exit with the result
	if len(exprs) > 0 {
		s, err := repl.New(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(2)
		}
		defer s.Close()
		if err := s.RunScript(strings.NewReader(strings.Join(exprs, "\n"))); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			os.Exit(2)
		}
		os.Exit(s.ExitCode())
	}

	s, err := repl.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	lr, interactive := repl.NewLineReader(s, historyPath(historyFile, noHistory), os.Stdin)
	s.SetInteractive(interactive)
	defer s.Close()
	err = s.Run(lr)
	lr.Close()
	// some terminals report no size at all, which rules out line editing.
	// Keep going with plain line input instead of bailing out
	if err == repl.ErrNoTerminal {
		s.SetInteractive(false)
		interactive = false
		err = s.RunScript(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	if !interactive {
		os.Exit(s.ExitCode())
	}
}

// loadLocal turns the file flags into a snapshot
func loadLocal(dataDir, factsPath, classesPath, lastRunPath string) (*puppet.Snapshot, error) {
	if len(dataDir) > 0 {
		return puppet.LoadSnapshotFiles(
			dataDir+"/"+puppet.HarnessFactsFile,
			dataDir+"/"+puppet.HarnessClassfile,
			dataDir+"/"+puppet.HarnessLastRunSummaryFile,
		)
	}
	if len(factsPath) == 0 {
		return nil, fmt.Errorf("need --facts (or --data-dir) to evaluate queries locally")
	}
	return puppet.LoadSnapshotFiles(factsPath, classesPath, lastRunPath)
}

func historyPath(flagValue string, noHistory bool) string {
	if noHistory {
		return ""
	}
	if len(flagValue) > 0 {
		return flagValue
	}
	return repl.DefaultHistoryFile()
}
