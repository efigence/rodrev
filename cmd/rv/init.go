package main

import (
	"os"
	"regexp"
	"strings"

	"github.com/XANi/go-yamlcfg"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	uuid "github.com/satori/go.uuid"
	"github.com/urfave/cli"
	"github.com/zerosvc/go-zerosvc"
)

func Init(c *cli.Context) (config.Config, common.Runtime) {
	cfgFiles := []string{
		"$HOME/.config/rodrev/client.conf",
		"/etc/rodrev/client.conf",
		"./cfg/client.yaml",
	}
	var cfg config.Config
	err := yamlcfg.LoadConfig(cfgFiles, &cfg)
	if err != nil {
		// if URL is unset
		if len(c.String("mqtt-url")) == 0 {
			log.Errorf("error loading config:", err)
		}
	}
	common.MergeCliConfig(&cfg, c)
	debug = c.GlobalBool("debug")
	quiet = c.GlobalBool("quiet")
	InitLog()

	log.Infof("config: %s", cfg.GetConfigPath())

	tr := zerosvc.NewTransport(
		zerosvc.TransportMQTT,
		cfg.MQAddress,
		zerosvc.TransportMQTTConfig{},
	)

	host, _ := os.Hostname()
	nodename := "rf-client-" + host
	node := zerosvc.NewNode(nodename, uuid.NewV4().String())
	err = tr.Connect()
	if err != nil {
		log.Panicf("can't connect: %s", err)
	}
	node.SetTransport(tr)

	runtime := common.Runtime{
		Node:     node,
		MQPrefix: cfg.MQPrefix,
		Log:      log,
	}
	outputMode := c.GlobalString("output-format")
	outputModeRe := regexp.MustCompile(
		"^" +
			strings.Join([]string{outCsv, outJson, outStderr}, "|") +
			"$")
	if !outputModeRe.MatchString(outputMode) {
		log.Panicf("output-format [%s] must match %s", outputMode, outputModeRe)
	}
	return cfg, runtime

}
