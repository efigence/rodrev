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
var quiet = false
var exit = make(chan bool, 1)

const (
	outStderr = "stderr"
	outCsv    = "csv"
	outJson   = "json"
)

func init() {
	InitLog()
}


func InitLog() {
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
	filterAll  := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool { return true })
	filterHighPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})
	filterQuiet := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.WarnLevel
	})
	filterInfo := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.InfoLevel
	})
	var logger *zap.Logger
	if debug {
		core := zapcore.NewCore(consoleEncoder, os.Stderr, filterAll)
		logger = zap.New(core).WithOptions(
			zap.Development(),
			zap.AddCaller(),
			zap.AddStacktrace(filterHighPriority),
		)
	} else if quiet {
		core := zapcore.NewCore(consoleEncoder, os.Stderr, filterQuiet)
		logger = zap.New(core).WithOptions(
			zap.AddCaller(),
		)
	} else {
		core := zapcore.NewCore(consoleEncoder, os.Stderr, filterInfo)
		logger = zap.New(core).WithOptions(
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
		cli.BoolFlag{Name: "quiet, q, s", Usage: "quiet/silent mode. will only show stderr warnings/errors"},
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
				cli.StringFlag{
					Name:        "filter,f,query",
					Usage:       "set a filter expression for nodes",
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
						if c.String("target") == "all" && c.GlobalDuration("random-delay") == 0 {
							log.Errorf("do not run all 'all' without delay, if you REALLY need to run all nodes at once set random-delay to '1s' ")
							os.Exit(1)
						}
						filter := c.GlobalString("filter")
						log.Warnf(filter)
						client.PuppetRun(&runtime, target, filter, c.GlobalDuration("random-delay"))
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
