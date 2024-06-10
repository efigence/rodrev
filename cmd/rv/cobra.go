package main

import (
	"fmt"
	"github.com/efigence/rodrev/cmd/rv/commands/puppet"
	"github.com/spf13/cobra"
	"os"
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
	Run:     Downtime,
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
	Run:   FenceRun,
}

var fenceStatusCmd = &cobra.Command{
	Use:   "status <node>",
	Short: "Check whether fencing is working on node",
	Run:   FenceStatus,
}
var ipsetCmd = &cobra.Command{
	Use:   "ipset",
	Short: "fencing commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var ipsetAddCmd = &cobra.Command{
	Use:   "add <group> <ipset> <addr>",
	Short: "add address to ipset group",
	Run:   IpsetAdd,
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

	downtimeCmd.PersistentFlags().String(
		"host",
		"",
		"hostname",
	)
}
func cobraInitCommands() {
	rootCmd.AddCommand(downtimeCmd)
	rootCmd.AddCommand(versionCmd)
	puppetCmd.AddCommand(puppetRunCmd)
	puppetCmd.AddCommand(puppetStatusCmd)
	rootCmd.AddCommand(puppetCmd)
	statusCmd.AddCommand(statusPuppetCmd)
	statusCmd.AddCommand(statusRodrevCmd)
	rootCmd.AddCommand(statusCmd)
	fenceCmd.AddCommand(fenceRunCmd)
	fenceCmd.AddCommand(fenceStatusCmd)
	rootCmd.AddCommand(fenceCmd)
	ipsetCmd.AddCommand(ipsetAddCmd)
	rootCmd.AddCommand(ipsetCmd)
}
