package daemon

import (
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/efigence/rodrev/util"
	uuid "github.com/satori/go.uuid"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"time"
)

type Daemon struct {
	node   *zerosvc.Node
	l      *zap.SugaredLogger
	prefix string
	fqdn   string
}

type Config struct {
	Logger      *zap.SugaredLogger
	MQTTAddress string
	Prefix      string
	Version     string
}

func New(cfg Config) (*Daemon, error) {
	var d Daemon
	d.prefix = cfg.Prefix
	// TODO load from cert
	d.fqdn = util.GetFQDN()
	d.l = cfg.Logger
	tr := zerosvc.NewTransport(zerosvc.TransportMQTT, cfg.MQTTAddress, zerosvc.TransportMQTTConfig{
		// cleanup retained heartbeat by sending empty message
		// no TTL in MQTTv3
		LastWillTopic:  cfg.Prefix + "heartbeat/" + d.fqdn,
		LastWillRetain: true,
	})

	// TODO save uuid somewhere
	d.node = zerosvc.NewNode(d.fqdn, uuid.NewV4().String())
	d.node.Info["fqdn"] = d.fqdn
	d.node.Info["version"] = cfg.Version

	d.node.Services["puppet"] = zerosvc.Service{
		Path:        "puppet",
		Description: "puppet management",
		Defaults:    nil,
	}
	err := tr.Connect()
	if err != nil {
		return nil, err
	}
	d.node.SetTransport(tr)

	runtime := common.Runtime{
		Node:     d.node,
		FQDN:     d.fqdn,
		MQPrefix: cfg.Prefix,
		Log:      cfg.Logger,
	}
	go d.heartbeat(time.Minute)

	pu, err := puppet.New(puppet.Config{
		Runtime: runtime,
	})
	if err != nil {
		return nil, err
	}
	pu.StartServer()
	go func() {
		for {
			ch, err := d.node.GetEventsCh(d.prefix + "puppet/#")
			if err != nil {
				d.l.Errorf("error connecting to channel: %s", err)
				time.Sleep(time.Second * 10)
				continue
			}
			err = pu.EventListener(ch)
			d.l.Errorf("plugin puppet exited: %s, reconnecting in 10s", err)
			time.Sleep(time.Second * 10)
		}
	}()
	return &d, nil
}

func (d *Daemon) heartbeat(interval time.Duration) {
	if interval == 0 {
		interval = time.Minute
	}
	for {
		ev := d.node.NewHeartbeat()
		t := time.Now().Add(interval * 3)
		ev.RetainTill = &t
		hbPath := d.prefix + "heartbeat/" + d.fqdn
		err := d.node.SendEvent(hbPath, ev)
		if err != nil {
			d.l.Warnf("could not send heartbeat: %s")
		}
		d.l.Debugf("HB sent to %s", hbPath)
		time.Sleep(interval)
	}

}
