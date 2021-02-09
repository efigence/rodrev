package main

import (
	"github.com/XANi/go-yamlcfg"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/daemon"
	"github.com/efigence/rodrev/hvminfo"
	"github.com/efigence/rodrev/util"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"fmt"
	"os/signal"
	"syscall"
	"time"
)

var rootCmd = &cobra.Command{
	Use: "rv",
	Short: "rodrev client",
	Long: "rodrev client/cli",
	Run:  func(cmd *cobra.Command, args []string) {
		c := cmd.Flags()
		if len(common.StringOrPanic(c.GetString("profile-addr"))) > 0 {
			go func() {
				log.Errorf("error starting debug port: %s", http.ListenAndServe(
					common.StringOrPanic(c.GetString("profile-addr")), nil))
			}()
		}
		cfgFiles := []string{
			"/etc/rodrev/server.conf",
			"./cfg/server-local.yaml",
			"./cfg/server.yaml",
		}
		userCfg, err := c.GetString("config")
		if err == nil && len(userCfg) > 0 {
			if _, err := os.Stat(userCfg); os.IsNotExist(err) {
				log.Panicf("config file %s does not exist", userCfg)
			}
			cfgFiles = append([]string{userCfg}, cfgFiles...)
		}

		var cfg config.Config
		err = yamlcfg.LoadConfig(cfgFiles, &cfg)
		if err != nil {
			log.Errorf("error loading config: %s",err)
		} else {
			log.Infof("loaded config from %s", cfg.GetConfigPath())
		}
		common.MergeCliConfig(&cfg, cmd)
		log.Warnf("%+v", cfg)

		debug = common.BoolOrPanic(c.GetBool("debug"))

		// reinit logger with cli settings
		setupLogger()
		cfg.Logger = log
		cfg.Version = version
		log.Debugf("MQ server url %s", cfg.MQAddress)
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)

		go func() {
			for sig := range hup {
				println(sig)
				log.Warnf("got HUP, exiting in 1 minute")
				time.Sleep(time.Minute)
				exit <- 0
			}
		}()

		log.Infof("Starting rodrev version: %s",  version)
		log.Infof("FQDN: %s", util.GetFQDN())
		if debug {
			cfg.Debug = debug
		}
		if cfg.HVMInfoServer != nil {
			serverCfg := *cfg.HVMInfoServer
			serverCfg.Info = hvminfo.HVMInfo{}.Default()
			serverCfg.Logger = log
			go hvminfo.Run(serverCfg)
		}
		d, err := daemon.New(cfg)

		if err != nil {
			log.Errorf("error starting daemon: %s", err)
			exit <- 1
		}

		_ = d
},}
var versionCmd = &cobra.Command{
	Use: "version",
	Short: "show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
		os.Exit(0)
	},
}

func cobraInit() {
	cobraInitFlags()
	cobraInitCommands()
}
func cobraDefaultString(env string,defaultValue string) string {
	e := os.Getenv(env)
	if e == "" {
		return defaultValue
	} else {
		return e
	}
}
func cobraInitFlags() {
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
	rootCmd.PersistentFlags().String(
		"profile-addr",
		"",
		"run profiler under this addr. example: localhost:6060",
	)
	rootCmd.PersistentFlags().StringP(
		"config",
		"c",
		"",
		"config file",
	)
}


func cobraInitCommands() {
	rootCmd.AddCommand(versionCmd)
}

