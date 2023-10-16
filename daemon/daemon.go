package daemon

import (
	"github.com/XANi/goneric"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/downtime"
	"github.com/efigence/rodrev/plugin/fence"
	"github.com/efigence/rodrev/plugin/ipset"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/efigence/rodrev/query"
	"github.com/efigence/rodrev/util"
	uuid "github.com/satori/go.uuid"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"time"
)

type Daemon struct {
	node    *zerosvc.Node
	runtime *common.Runtime
	query   *query.Engine

	l      *zap.SugaredLogger
	prefix string
	fqdn   string
}

func New(cfg config.Config) (*Daemon, error) {
	var d Daemon
	d.prefix = cfg.MQPrefix
	// TODO load from cert
	d.fqdn = util.GetFQDN()
	d.l = cfg.Logger
	tr := zerosvc.NewTransport(zerosvc.TransportMQTT, cfg.MQAddress, zerosvc.TransportMQTTConfig{
		// cleanup retained heartbeat by sending empty message
		// no TTL in MQTTv3
		LastWillTopic:  cfg.MQPrefix + "heartbeat/" + d.fqdn,
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

	runtime := &common.Runtime{
		Node:     d.node,
		FQDN:     d.fqdn,
		MQPrefix: cfg.MQPrefix,
		Log:      cfg.Logger,
		Metadata: cfg.NodeMeta,
		Cfg:      cfg,
	}
	d.runtime = runtime
	d.query = query.NewQueryEngine(runtime)
	go d.heartbeat(time.Minute)

	pu, err := puppet.New(puppet.Config{
		Runtime: runtime,
		Query:   d.query,
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
	if cfg.Fence.Enabled {
		cfg.Fence.Logger = d.l
		f, err := fence.New(runtime, cfg.Fence)
		_ = f
		if err != nil {
			// TODO alert/fail somehow
			d.l.Errorf("starting fencing failed: %s", err)
		}
		d.l.Infof("starting fencing plugin")
		go func() {
			for {
				ch, err := d.node.GetEventsCh(d.prefix + "fence/" + d.fqdn)
				if err != nil {
					d.l.Errorf("error connecting to channel: %s", err)
					time.Sleep(time.Second * 10)
					continue
				}
				err = f.EventListener(ch)
				d.l.Errorf("plugin fence exited: %s, reconnecting in 10s", err)
				time.Sleep(time.Second * 10)
			}
		}()
	}
	if len(cfg.IPSet.Sets) > 0 {
		d.l.Infof("starting ipset management with sets [%+v]", goneric.MapSliceKey(cfg.IPSet.Sets))
		cfg.IPSet.Logger = d.l.Named("ipset")
		ipset, err := ipset.New(runtime, cfg.IPSet)
		if err != nil {
			// TODO alert/fail somehow
			d.l.Errorf("starting ipset failed: %s", err)
		} else {
			for _, setcfg := range cfg.IPSet.Sets {
				go func(setname config.IPSet) {
					for {
						topic := d.prefix + "ipset/" +
							setname.BroadcastGroup +
							"/" + setname.Name

						ch, err := d.node.GetEventsCh(topic)
						if err != nil {
							d.l.Errorf("error getting event channel for ipset [%s]: %s", err)
							time.Sleep(time.Second * 60)
							continue
						} else {
							d.l.Infof("subscribing to %s", topic)
						}
						err = ipset.EventListener(ch, setname.Name)
						if err != nil {
							d.l.Errorf("error on ipset [%s] event listener: %s", err)
						}
						time.Sleep(time.Second * 10)
					}
				}(setcfg)
			}
		}
	}
	if len(cfg.IcingaAPIURL) > 0 {
		d.l.Infof("starting downtime plugin [%s]", cfg.IcingaAPIURL)
		api, err := downtime.NewDowntimeServer(downtime.Config{
			Icinga2URL:  cfg.IcingaAPIURL,
			Icinga2User: cfg.IcingaAPIUser,
			Icinga2Pass: cfg.IcingaAPIPass,
		})
		if err != nil {
			d.l.Errorf("error initializing icinga api: %w", err)
		} else {
			for {
				ch, err := d.node.GetEventsCh(d.prefix + "downtime/")
				if err != nil {
					d.l.Errorf("error initializing icinga api channel: %w", err)
					goto endapi
				}
				api.Run(ch)
				d.l.Infof("restarting downtime api channel")
				time.Sleep(time.Second * 60)
			}

		}
	}
endapi:

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
