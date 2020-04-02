package main

import (
	"github.com/efigence/rodrev/daemon"
	"github.com/efigence/rodrev/util"
	"github.com/urfave/cli"
)

func RunDaemon(c *cli.Context) {
	log.Infof("FQDN: %s", util.GetFQDN())
	d,err := daemon.New(daemon.Config{
		MQTTAddress: c.String("mqtt-url"),
		Logger:      log,
	})
	if err != nil {
		log.Errorf("error starting daemon: %s", err)
		exit <- 1
	}
	_ = d

}