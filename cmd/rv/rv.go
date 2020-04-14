package main

import (
	"github.com/efigence/rodrev/client"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var version string
var log *zap.SugaredLogger
var debug = true
var exit = make(chan bool, 1)

const (
	outStderr = "stderr"
	outCsv    = "csv"
	outJson   = "json"
)

func init() {
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	// naive systemd detection. Drop timestamp if running under it
	// if os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != "" {
	// 	consoleEncoderConfig.TimeKey = ""
	// }
	consoleEncoderConfig.TimeKey = ""
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
	consoleStderr := zapcore.Lock(os.Stderr)
	_ = consoleStderr

	// if needed point differnt priority log to different place
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
	app.Name = "rodrev"
	app.Usage = "rodrev client"
	app.Description = "send commands to and read state from daemon"
	app.Version = version
	app.HideHelp = true
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "help, h", Usage: "show help"},
		cli.StringFlag{
			Name:   "mqtt-url",
			Usage:  "URL for the MQ server. Use tls:// to enable encryption (default: tcp://mqtt:mqtt@127.0.0.1:1883)",
			EnvVar: "RF_MQTT_URL",
		},
		cli.StringFlag{
			Name:  "output-format,o,out",
			Usage: "Output format: stderr(human readable),csv,json",
			Value: "stderr",
		},
	}
	app.Action = func(c *cli.Context) error {
		cli.ShowAppHelp(c)
		return nil
	}
	app.Commands = []cli.Command{
		{
			Name:    "puppet",
			Aliases: []string{"p", "pu"},
			Usage:   "puppet management (run/status/etc)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "target",
					Usage: "node to run puppet on. 'all' to run on all nodes (SET DELAY)",
				},
				cli.DurationFlag{
					Name:  "random-delay,delay",
					Usage: "add random delay to each run. Use when running many at once",
				},
			},
			Subcommands: []cli.Command {
				{
					Name: "run",
					Usage: "run puppet on one or more machines. Needs --target. Specify --target all to run on all discovered ones",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "target",
							Usage: "node to run puppet on. 'all' to run on all nodes (SET DELAY)",
							Required: true,
						},
					},
					Action : func(c *cli.Context) error {
						cfg, runtime := Init(c)
						_ = cfg
						target := c.String("target")
						if len(target) == 0 {
							log.Warn("need --target parameter")
							os.Exit(1)
						}
						if c.String("target") == "all" && c.Duration("random-delay") == 0 {
							log.Errorf("do not run all 'all' without delay, if you REALLY need to run all nodes at once set random-delay to '1s' ")
							os.Exit(1)
						}
						client.PuppetRun(&runtime, target, c.Duration("random-delay"))
						log.Warnf("running puppet on %s", c.String("target"))
						return nil
					},
				},
				{
					Name:        "status",
					Usage:       "status",
					Description: "display status of last puppet run",
					Action:      StatusPuppet,
				},
			},
			Action: func(c *cli.Context) error {
				cli.ShowAppHelp(c)
				return nil
			},
		},
		{
			Name:    "status",
			Aliases: []string{"s"},
			Usage:   "get status",
			Subcommands: []cli.Command{
				{
					Name:   "puppet",
					Usage:  "puppet status",
					Action: StatusPuppet,
				},
				{
					Name:   "discovery",
					Usage:  "node discovery status",
					Action: StatusRodrev,
				},
			},
			Action: func(c *cli.Context) {
				cli.ShowAppHelp(c)
				StatusRodrev(c)
			},
		},
	}
	// to sort do that
	//sort.Sort(cli.FlagsByName(app.Flags))
	//sort.Sort(cli.CommandsByName(app.Commands))
	app.Run(os.Args)
}
