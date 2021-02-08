package main

import (
	"fmt"
	"github.com/efigence/rodrev/client"
	"github.com/spf13/cobra"
	"os"
	"time"
)
// Root
var rootCmd = &cobra.Command{
	Use: "rv",
	Short: "rodrev client",
	Long: "rodrev client/cli",
	Run:  func(cmd *cobra.Command, args []string) {
		cmd.Help()
		os.Exit(1)
	},
}

var versionCmd = &cobra.Command{
	Use: "version",
	Short: "show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
		os.Exit(0)
	},
}


// Puppet
var puppetCmd = &cobra.Command{
	Use: "puppet",
	Short: "puppet management (run/status/etc)",
//	Args: cobra.MinimumNArgs(1),
	Run:  func(cmd *cobra.Command, args []string) {
		cmd.Help()
		os.Exit(1)
	},
}

var puppetRunCmd = &cobra.Command{
	Use: "run",
	Short: "run puppet on one or more machines. Needs --target. Specify --target all to run on all discovered ones",
	Run:  func(cmd *cobra.Command, args []string) {
		cfg, runtime := Init(cmd)
		c := cmd.Flags()
		_ = cfg
		target := stringOrPanic(c.GetString("node"))
		if len(target) == 0 {
			target = stringOrPanic(c.GetString("target"))
		}
		randomDelay :=  durationOrPanic(c.GetDuration("random-delay"))
		if len(target) == 0 {
			log.Warn("need --target parameter")
			os.Exit(1)
		}
		if stringOrPanic(c.GetString("node")) == "all" &&
			randomDelay == 0 &&
			len(stringOrPanic(c.GetString("filter"))) > 3 {
			randomDelay = time.Second
		}
		if stringOrPanic(c.GetString("node")) == "all" && randomDelay == 0 {
			log.Errorf("do not run all 'all' without delay, if you REALLY need to run all nodes at once set random-delay to '1s' ")
			cmd.Help()
			os.Exit(1)
		}
		filter := stringOrPanic(c.GetString("filter"))
		log.Warnf("filter query: %s", filter)
		client.PuppetRun(&runtime, target, filter, randomDelay)
		log.Warnf("sending  puppet run request to %s", stringOrPanic(c.GetString("node")))
	},
}

var puppetStatusCmd = &cobra.Command{
	Use: "status",
	Short: "display status of last puppet run",
	Run:  func(cmd *cobra.Command, args []string) {
		StatusPuppet(cmd)
	},
}

// Status
var statusCmd = &cobra.Command{
	Use: "status",
	Short: "get status",
	Run:  func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var statusPuppetCmd = &cobra.Command{
	Use: "puppet",
	Short: "Puppet Status",
	Run:  func(cmd *cobra.Command, args []string) {
		StatusPuppet(cmd)

	},
}

var statusRodrevCmd = &cobra.Command{
	Use: "rodrev",
	Short: "Rodrev status",
	Run:  func(cmd *cobra.Command, args []string) {
		StatusRodrev(cmd)
	},
}

var fenceCmd = &cobra.Command{
	Use:   "fence",
	Short: "fencing commands",
	Run:  func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}


var fenceRunCmd = &cobra.Command{
	Use:   "run <node>",
	Short: "Run fencing on node specified as parameter",
	Run:   RunFence,
}

func cobraDefaultString(env string,defaultValue string) string {
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
}
func cobraInitCommands() {
	rootCmd.AddCommand(versionCmd)
	puppetCmd.AddCommand(puppetRunCmd)
	puppetCmd.AddCommand(puppetStatusCmd)
	rootCmd.AddCommand(puppetCmd)
	statusCmd.AddCommand(statusPuppetCmd)
	statusCmd.AddCommand(statusRodrevCmd)
	rootCmd.AddCommand(statusCmd)
	fenceCmd.AddCommand(fenceRunCmd)
	rootCmd.AddCommand(fenceCmd)
}
