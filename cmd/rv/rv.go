package main

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/efigence/rodrev/client"
	"github.com/efigence/rodrev/common"
	"github.com/urfave/cli"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"sort"
)

var version string
var log *zap.SugaredLogger
var debug = true
var exit = make(chan bool, 1)

func init() {
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	// naive systemd detection. Drop timestamp if running under it
	// if os.Getenv("INVOCATION_ID") != "" || os.Getenv("JOURNAL_STREAM") != "" {
	// 	consoleEncoderConfig.TimeKey = ""
	// }
	consoleEncoderConfig.TimeKey=""
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
	app.Description = "rodrev client"
	app.Version = version
	app.HideHelp = true
	app.Flags = []cli.Flag{
		cli.BoolFlag{Name: "help, h", Usage: "show help"},
		cli.StringFlag{
			Name:   "mqtt-url",
			Value:  "tcp://mqtt:mqtt@127.0.0.1:1883",
			Usage:  "URL for the MQ server. Use tls:// to enable encryption",
			EnvVar: "RF_MQTT_URL",
		},
		cli.BoolFlag{
			Name: "service-discovery",
			Usage: "dump service discovery",
		},
		cli.BoolFlag{
			Name: "status-map",
			Usage: "puppet status",
		},
	}
	app.Action = func(c *cli.Context) error {
		if c.Bool("help") {
			cli.ShowAppHelp(c)
			os.Exit(1)
		}

		tr := zerosvc.NewTransport(
			zerosvc.TransportMQTT,
			c.String("mqtt-url"),
			zerosvc.TransportMQTTConfig{},
		)
		host,_ := os.Hostname()
		rn := make([]byte,4)
		rand.Read(rn)
		nodename := "rf-client" + host + hex.EncodeToString(rn)
		node := zerosvc.NewNode(nodename)
		err := tr.Connect()
		if err != nil {
			log.Panicf("can't connect: %s",err)
		}
		node.SetTransport(tr)

		runtime := common.Runtime{
			Node:     node,
			MQPrefix: "rv/",
			Log:      log,
		}

		if c.Bool("service-discovery") {
			log.Info("running service discovery")
			services, nodesActive, nodesStale, err := client.Discover(&runtime)
			if err != nil {
				log.Errorf("error running discovery: %s", err)
			}
			log.Infof("services:")
			for service, nodes := range services {
				log.Infof("  %s:", service)
				sort.Slice(nodes, func(i, j int) bool { return nodes[i].FQDN < nodes[j].FQDN })
				for  _, node := range  nodes {
					log.Infof("    %s:",node.FQDN)
				}
			}
			_ = nodesActive // not uset yet
			for name, node := range nodesStale {
				log.Warnf("node %s is stale: %+v", name, node)
			}
		}
		if c.Bool("status-map") {
			log.Info("puppet status")
			client.PuppetStatus(&runtime)
		}


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
	//sort.Sort(cli.FlagsByName(app.Flags))
	//sort.Sort(cli.CommandsByName(app.Commands))
	app.Run(os.Args)
}
