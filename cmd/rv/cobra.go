package main

import (
	"fmt"
	"github.com/efigence/rodrev/cmd/rv/commands/downtime"
	"github.com/efigence/rodrev/cmd/rv/commands/fence"
	"github.com/efigence/rodrev/cmd/rv/commands/ipset"
	"github.com/efigence/rodrev/cmd/rv/commands/puppet"
	"github.com/efigence/rodrev/cmd/rv/commands/query"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"time"
)

// Root
var rootCmd = &cobra.Command{
	Use:   "rv",
	Short: "rodrev client",
	Long:  "rodrev client/cli",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
		os.Exit(1)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
		os.Exit(0)
	},
}

// Puppet
var puppetCmd = &cobra.Command{
	Use:   "puppet",
	Short: "puppet management (run/status/etc)",
	//	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
		os.Exit(1)
	},
}

var downtimeCmd = &cobra.Command{
	Use:     "downtime",
	Short:   "set downtime on server",
	Long:    "",
	Example: "downtime 8h | downtime --host abc 20m",
	Run:     downtime.Downtime,
}

var puppetRunCmd = &cobra.Command{
	Use:   "run",
	Short: "run puppet on one or more machines. Needs --target. Specify --target all to run on all discovered ones",
	Run: func(cmd *cobra.Command, args []string) {
		puppet.Run(cmd)
	},
}

var puppetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "display status of last puppet run",
	Run: func(cmd *cobra.Command, args []string) {
		puppet.Status(cmd)
	},
}
var puppetFactCmd = &cobra.Command{
	Use:   "fact",
	Short: "show fact value",
	Run:   puppet.Fact,
}

var queryCmd = &cobra.Command{
	Use:     "query [expression]",
	Aliases: []string{"q"},
	Short:   "interactive REPL for testing filter expressions (rv puppet --filter)",
	Long: "Interactive REPL for building and testing query/filter expressions.\n" +
		"By default queries are sent to the cluster and matching nodes are listed;\n" +
		"pass --facts/--classes (or --data-dir) to evaluate them locally instead.\n" +
		"Use :help for commands and :syntax for the query language.",
	Example: "  " + strings.Join([]string{
		`query                                                  interactive, on the cluster`,
		`query '(== (class "nginx") true)'                       one shot, exit 0 when something matched`,
		`query -o json -e '(== (fact "virtual") "kvm")'          machine readable`,
		`query --data-dir t-data                                 offline, against example data`,
		`query --facts /var/lib/puppet/facts.yaml --classes /var/lib/puppet/state/classes.txt`,
	}, "\n  "),
	Run: query.Query,
}

// Status
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "get status",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var statusPuppetCmd = &cobra.Command{
	Use:   "puppet",
	Short: "Puppet Status",
	Run: func(cmd *cobra.Command, args []string) {
		puppet.Status(cmd)

	},
}

var statusRodrevCmd = &cobra.Command{
	Use:   "rodrev",
	Short: "Rodrev status",
	Run: func(cmd *cobra.Command, args []string) {
		StatusRodrev(cmd)
	},
}

var fenceCmd = &cobra.Command{
	Use:   "fence",
	Short: "fencing commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var fenceRunCmd = &cobra.Command{
	Use:   "run <node>",
	Short: "Run fencing on node specified as parameter",
	Run:   fence.Run,
}

var fenceStatusCmd = &cobra.Command{
	Use:   "status <node>",
	Short: "Check whether fencing is working on node",
	Run:   fence.Status,
}
var ipsetCmd = &cobra.Command{
	Use:   "ipset",
	Short: "ipset control",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var ipsetAddCmd = &cobra.Command{
	Use:   "add <group> <ipset> <addr>",
	Short: "add address to ipset group",
	Run:   ipset.Add,
}
var ipsetDeleteCmd = &cobra.Command{
	Use:   "delete <group> <ipset> <addr>",
	Short: "delete address from ipset group",
	Run:   ipset.Delete,
}

func cobraDefaultString(env string, defaultValue string) string {
	e := os.Getenv(env)
	if e == "" {
		return defaultValue
	} else {
		return e
	}
}

func cobraInit() {
	cobraInitFlags()
	cobraInitCommands()
}

func cobraInitFlags() {
	rootCmd.PersistentFlags().BoolP(
		"quiet",
		"q",
		false,
		"quiet/silent mode. will only show stderr warnings/errors",
	)
	rootCmd.PersistentFlags().BoolP(
		"debug",
		"d",
		false,
		"Debug mode",
	)
	rootCmd.PersistentFlags().String(
		"mqtt-url",
		cobraDefaultString("RF_MQTT_URL", ""), // do not put default there, it is in MergeCliConfig
		"URL for the MQ server. Use tls:// to enable encryption (default: tcp://mqtt:mqtt@127.0.0.1:1883)",
	)
	rootCmd.PersistentFlags().StringP(
		"output-format",
		"o",
		"stderr",
		"Output format: stderr(human readable),csv,json",
	)
	rootCmd.PersistentFlags().StringP(
		"config",
		"c",
		"",
		"config file",
	)
	//
	puppetCmd.PersistentFlags().StringP(
		"node",
		"n",
		"all",
		"node to run puppet on. 'all' to run on all nodes (SET DELAY or else you can DDoS your own cluster)",
	)
	puppetCmd.PersistentFlags().String(
		"target",
		"all",
		"node to run puppet on. deprecated",
	)
	puppetCmd.PersistentFlags().StringP(
		"filter",
		"f",
		"",
		"set a filter expression for nodes",
	)
	puppetCmd.PersistentFlags().DurationP(
		"random-delay",
		"t",
		0,
		"add random delay to each run. Use when running many at once",
	)
	puppetCmd.PersistentFlags().Bool(
		"noop",
		false,
		"run puppet with  --noop",
	)

	downtimeCmd.PersistentFlags().String(
		"host",
		"",
		"hostname",
	)
	//
	queryCmd.Flags().StringArrayP(
		"eval",
		"e",
		[]string{},
		"evaluate expression (or :command) and exit. Can be repeated",
	)
	queryCmd.Flags().String(
		"facts",
		"",
		"facts.yaml to use. Implies local evaluation; in cluster mode it only feeds completion and :fact",
	)
	queryCmd.Flags().String(
		"classes",
		"",
		"classes.txt to use. Implies local evaluation",
	)
	queryCmd.Flags().String(
		"last-run-summary",
		"",
		"last_run_summary.yaml to use (optional)",
	)
	queryCmd.Flags().String(
		"data-dir",
		"",
		"directory with facts.yaml/classes.txt/last_run_summary.yaml. Implies local evaluation",
	)
	queryCmd.Flags().Bool(
		"local",
		false,
		"evaluate locally, never connect to the MQ",
	)
	queryCmd.Flags().Duration(
		"timeout",
		time.Second*4,
		"how long to wait for query results",
	)
	queryCmd.Flags().String(
		"history",
		cobraDefaultString("RV_QUERY_HISTORY", ""),
		"query history file (default: ~/.rv_query_history)",
	)
	queryCmd.Flags().Bool(
		"no-history",
		false,
		"do not read or write the query history file",
	)
}
func cobraInitCommands() {
	rootCmd.AddCommand(downtimeCmd)
	rootCmd.AddCommand(versionCmd)
	puppetCmd.AddCommand(puppetRunCmd)
	puppetCmd.AddCommand(puppetStatusCmd)
	puppetCmd.AddCommand(puppetFactCmd)
	rootCmd.AddCommand(puppetCmd)
	statusCmd.AddCommand(statusPuppetCmd)
	statusCmd.AddCommand(statusRodrevCmd)
	rootCmd.AddCommand(statusCmd)
	fenceCmd.AddCommand(fenceRunCmd)
	fenceCmd.AddCommand(fenceStatusCmd)
	rootCmd.AddCommand(fenceCmd)
	ipsetCmd.AddCommand(ipsetAddCmd)
	ipsetCmd.AddCommand(ipsetDeleteCmd)
	rootCmd.AddCommand(ipsetCmd)
	rootCmd.AddCommand(queryCmd)
}
