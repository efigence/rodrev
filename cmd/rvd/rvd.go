package main

import (
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/daemon"
	"github.com/efigence/rodrev/util"
	"github.com/XANi/go-yamlcfg"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/url"
	"os"
	"sort"
	"strings"
)

var version string
var log *zap.SugaredLogger
var debug = false
var exit = make(chan int, 1)



func init() {
	setupLogger()
}

func setupLogger() {
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	// naive systemd detection. Drop timestamp if running under it
	if os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != "" {
		consoleEncoderConfig.TimeKey = ""
	}
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
	consoleStderr := zapcore.Lock(os.Stderr)
	_ = consoleStderr

	// if needed point different priority log to different place
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})
	lowPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl < zapcore.ErrorLevel
	})
	infoPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.InfoLevel
	})
	var core zapcore.Core
	if debug {
		core = zapcore.NewTee(
			zapcore.NewCore(consoleEncoder, os.Stderr, lowPriority),
			zapcore.NewCore(consoleEncoder, os.Stderr, highPriority),
		)
	} else {
		core = zapcore.NewCore(consoleEncoder, os.Stderr, infoPriority)
	}
	logger := zap.New(core)
	if debug {
		logger = logger.WithOptions(
			zap.Development(),
			zap.AddCaller(),
			zap.AddStacktrace(highPriority),
		)
	} else {
		logger = logger.WithOptions(
			zap.AddCaller(),
		)
	}
	log = logger.Sugar()
}


func main() {

	app := cli.NewApp()
	app.Name = "rvd"
	app.Description = "Rodrev server"
	app.Version = version
	app.HideHelp = true
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "help, h", Usage: "show help"},
		cli.BoolFlag{Name: "debug, d", Usage: "debug"},
		cli.StringFlag{
			Name:   "mqtt-url",
			Usage:  "URL for the MQ server. Use tls:// to enable encryption (default tcp://mqtt:mqtt@127.0.0.1:1883)",
			EnvVar: "RF_MQTT_URL",
		},
	}
	app.Action = func(c *cli.Context) error {

		cfgFiles := []string{
			"/etc/rodrev/server.conf",
			"./cfg/server.yaml",
		}
		var cfg config.Config
		err := yamlcfg.LoadConfig(cfgFiles, &cfg)
		if err != nil {
			log.Errorf("error loading config")
		} else {
			log.Infof("loaded config from %s", cfg.GetConfigPath())
		}
		if len(c.String("mqtt-url")) > 0 {
			cfg.MQAddress = c.String("mqtt-url")
		}
		if len(cfg.MQAddress) == 0 {
			cfg.MQAddress = "tcp: // mqtt:mqtt@127.0.0.1:1883"
		}
		if !strings.Contains(cfg.MQAddress,"/?") {
			cfg.MQAddress = cfg.MQAddress + "/?"
		}

		if !strings.Contains(cfg.MQAddress,"ca=") && len(cfg.CA) > 0 {
			cfg.MQAddress = cfg.MQAddress + "&ca=" + url.QueryEscape(cfg.CA)
		}
		if !strings.Contains(cfg.MQAddress,"cert=") && len(cfg.ClientCert) > 0 {
			cfg.MQAddress = cfg.MQAddress + "&cert=" + url.QueryEscape(cfg.ClientCert)
		}

		debug = c.Bool("debug")

		if c.Bool("help") {
			cli.ShowAppHelp(c)
			os.Exit(1)
		}
		// reinit logger with cli settings
		setupLogger()
		log.Debugf("MQ server url %s",cfg.MQAddress)

		log.Infof("Starting %s version: %s", app.Name, version)
			log.Infof("FQDN: %s", util.GetFQDN())
		d, err := daemon.New(daemon.Config{
			MQTTAddress: cfg.MQAddress,
			Logger:      log,
			Version: version,
			Prefix:  cfg.MQPrefix,
		})
		if err != nil {
			log.Errorf("error starting daemon: %s", err)
			exit <- 1
		}
		_ = d

		return nil
	}
	app.Commands = []cli.Command{
		{
			Name:    "rem",
			Aliases: []string{"a"},
			Usage:   "example cmd",
			Action: func(c *cli.Context) error {
				log.Warn("running example cmd")
				return nil
			},
		},
		{
			Name:    "add",
			Aliases: []string{"a"},
			Usage:   "example cmd",
			Action: func(c *cli.Context) error {
				log.Warn("running example cmd")
				return nil
			},
		},
	}


	// to sort do that
	sort.Sort(cli.FlagsByName(app.Flags))
	sort.Sort(cli.CommandsByName(app.Commands))
	app.Run(os.Args)
	os.Exit(<- exit)
}
