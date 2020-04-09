package main

import (
	"os"

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
		if len(c.String("mqtt-ur")) == 0 {
			log.Errorf("error loading config:", err)
		}
	}
	common.MergeCliConfig(&cfg, c)
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
	return cfg, runtime

}
