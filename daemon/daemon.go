package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/efigence/rodrev/util"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"time"
)

type Daemon struct {
	node *zerosvc.Node
	l *zap.SugaredLogger
	prefix string
	fqdn string
}

type Config struct {
	Logger *zap.SugaredLogger
	MQTTAddress string
	Prefix string
}

func New(cfg Config) (*Daemon, error) {
	var d Daemon
	tr := zerosvc.NewTransport(zerosvc.TransportMQTT,cfg.MQTTAddress,zerosvc.TransportMQTTConfig{})
	d.prefix = "rf/"
	d.fqdn = util.GetFQDN()
	d.l = cfg.Logger
	rn := make([]byte,4)
    rand.Read(rn)
	d.node = zerosvc.NewNode("rf-" + d.fqdn + "-" +hex.EncodeToString(rn))
	d.node.Services["puppet"] = zerosvc.Service{
		Path:        "puppet",
		Description: "puppet management",
		Defaults:    nil,
	}
	err := tr.Connect()
	if err != nil {
		return nil,err
	}
	d.node.SetTransport(tr)
	go d.heartbeat(time.Minute)

	pu,err  := puppet.New(puppet.Config{
		Logger:d.l,
		Node:d.node,
	})
	if err != nil {
		return nil, err
	}
	pu.StartServer()
	go func() {
		for {
			ch, err := d.node.GetEventsCh(d.prefix + "puppet/#")
			if err != nil {
				d.l.Errorf("error connecting to channel: %s",err)
				time.Sleep(time.Second * 10)
				continue
			}
			err = pu.EventListener(ch)
			d.l.Errorf("plugin puppet exited: %s, reconnecting in 10s",err)
			time.Sleep(time.Second * 10)
		}
	}()
	return &d,nil
}

func(d *Daemon) heartbeat(interval time.Duration) {
	if interval == 0 {
		interval = time.Minute
	}
	for {
		ev := d.node.NewHeartbeat()
		t := time.Now().Add(interval * 3)
		ev.RetainTill = &t
		hbPath := d.prefix + "heartbeat/" + d.fqdn
		err := d.node.SendEvent(hbPath,ev)
		if err != nil {
			d.l.Warnf("could not send heartbeat: %s")
		}
		d.l.Debugf("HB sent to %s", hbPath)
		time.Sleep(interval)
	}

}


