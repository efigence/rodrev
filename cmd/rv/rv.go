package main

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"github.com/efigence/rodrev/client"
	"github.com/efigence/rodrev/common"
	uuid "github.com/satori/go.uuid"
	"github.com/urfave/cli"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var version string
var log *zap.SugaredLogger
var debug = true
var exit = make(chan bool, 1)

const (
	outStderr = "stderr"
	outCsv = "csv"
	outJson = "json"
)


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
		cli.StringFlag{
			Name:  "output-format,o,out",
			Usage: "Output format: stderr(human readable),csv,json",
			Value: "stderr",
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
		nodename := "rf-client-" + host
		node := zerosvc.NewNode(nodename,uuid.NewV4().String())
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
		outputMode := c.String("output-format")
		outputModeRe := regexp.MustCompile(
			"^" +
				strings.Join([]string{outCsv,outJson,outStderr},"|") +
				"$")
		if !outputModeRe.MatchString(c.String("output-format")) {
			log.Panicf("output-format [%s] must match %s",outputMode, outputModeRe)
		}

		if c.Bool("service-discovery") {
			log.Info("running service discovery")
			services, nodesActive, nodesStale, err := client.Discover(&runtime)
			if err != nil {
				log.Errorf("error running discovery: %s", err)
			}
			log.Infof("services:")
			switch outputMode {
			case outStderr:
				for service, nodes := range services {
					log.Infof("  %s:", service)
					sort.Slice(nodes, func(i, j int) bool { return nodes[i].FQDN < nodes[j].FQDN })
					for _, node := range nodes {
						log.Infof("    %s:", node.FQDN)
					}
				}
				for name, node := range nodesStale {
					log.Warnf("node %s is stale: %+v", name, node)
				}

			case outCsv:
				csvW := csv.NewWriter(os.Stdout)
				csvW.Write([]string{"fqdn", "last_update", "version", "service", "active"})
				for _, info := range nodesActive {
					csvW.Write([]string{
						info.FQDN,
						info.LastUpdate.Format(time.RFC3339),
						info.DaemonVersion,
						strings.Join(info.Services, ","),
						"1",
					})
				}
				for _, info := range nodesStale {
					csvW.Write([]string{
						info.FQDN,
						info.LastUpdate.Format(time.RFC3339),
						info.DaemonVersion,
						strings.Join(info.Services, ","),
						"0",
					})
				}
				csvW.Flush()
			case outJson:
				outJ := make(map[string]interface{}, 0)
				outJ["services"] = services
				outJ["nodes_active"] = nodesActive
				outJ["nodes_stale"] = nodesStale
				err = json.NewEncoder(os.Stdout).Encode(&outJ)
				if err != nil {
					log.Errorf("error encoding node data: %s", err)
				}
			}
		}
		if c.Bool("status-map") {
			status := client.PuppetStatus(&runtime)

			switch outputMode {
			case outStderr:
				log.Info("puppet status")
				for node, summary := range status {
					log.Infof("%s: %s, changes: %d/%d",
						node,
						summary.Version.Config,
						summary.Resources.Changed,
		    			summary.Resources.Total,
					)
				}
			case outCsv:
				csvW := csv.NewWriter(os.Stdout)
				csvW.Write([]string{"fqdn", "puppet_version", "config_version", "last_run", "changed", "total", "duration"})
				for node, summary := range status {
					totalDuration := "0"
					if v, ok := summary.Timing.Duration["total"]; ok {
						totalDuration = strconv.FormatFloat(v, 'f', 2, 64)
					}
					csvW.Write([]string{
						node,
						summary.Version.Puppet,
						summary.Version.Config,
						time.Unix(int64(summary.Timing.LastRun), 0).Format(time.RFC3339),
						strconv.Itoa(summary.Resources.Changed),
						totalDuration,
					})
				}
				csvW.Flush()
			case outJson:
				err = json.NewEncoder(os.Stdout).Encode(&status)
				if err != nil {
					log.Errorf("error encoding node data: %s", err)
				}
			}
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
