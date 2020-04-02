package main

import (
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"sort"
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
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, os.Stderr, lowPriority),
		zapcore.NewCore(consoleEncoder, os.Stderr, highPriority),
	)
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
	app.Name = "vpd"
	app.Description = "Rodrev server"
	app.Version = version
	app.HideHelp = true
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "help, h", Usage: "show help"},
		cli.BoolFlag{Name: "debug d", Usage: "debug"},
		cli.StringFlag{
			Name:   "mqtt-url",
			Value:  "tcp://mqtt:mqtt@127.0.0.1:1883",
			Usage:  "URL for the MQ server. Use tls:// to enable encryption",
			EnvVar: "RF_MQTT_URL",
		},
	}
	app.Action = func(c *cli.Context) error {
		debug = c.Bool("debug")

		if c.Bool("help") {
			cli.ShowAppHelp(c)
			os.Exit(1)
		}
		// reinit logger with cli settings
		setupLogger()

		log.Infof("Starting %s version: %s", app.Name, version)
		log.Infof("var example %s", c.GlobalString("url"))
		RunDaemon(c)
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
