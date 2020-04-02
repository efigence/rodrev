package daemon

import (
	"crypto/rand"
	"encoding/hex"
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
	d.prefix = "rf"
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
	go d.heartbeat(time.Second * 10)
	return &d,nil
}

func(d *Daemon) heartbeat(interval time.Duration) {
	if interval == 0 {
		interval = time.Minute
	}
	for {
		ev := d.node.NewHeartbeat()
		ev.RetainTill = time.Now().Add(interval * 3)

		err := d.node.SendEvent(d.prefix + "/heartbeat/" + d.fqdn,ev)
		if err != nil {
			d.l.Warnf("could not send heartbeat: %s")
		}
		d.l.Infof("HB sent")
		time.Sleep(interval)
	}

}

