package daemon

import (
	"fmt"
	"github.com/XANi/goneric"
	"github.com/efigence/go-mon"
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/downtime"
	"github.com/efigence/rodrev/embedfs"
	"github.com/efigence/rodrev/plugin/fence"
	"github.com/efigence/rodrev/plugin/ipset"
	"github.com/efigence/rodrev/plugin/puppet"
	"github.com/efigence/rodrev/query"
	"github.com/efigence/rodrev/util"
	"github.com/efigence/rodrev/web"
	uuid "github.com/satori/go.uuid"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"time"
)

// heartbeatInterval is how often the daemon announces itself. Clients treat a
// heartbeat older than a few intervals as stale
const heartbeatInterval = time.Minute

type Daemon struct {
	node    *zerosvc.Node
	runtime *common.Runtime
	query   *query.Engine

	l        *zap.SugaredLogger
	prefix   string
	fqdn     string
	exitFunc func(reason string)
}

func New(cfg config.Config) (*Daemon, error) {
	var d Daemon
	d.prefix = cfg.MQPrefix
	// TODO load from cert
	d.fqdn = util.GetFQDN()
	d.exitFunc = cfg.ExitFunc
	d.l = cfg.Logger
	d.l.Infof("connecting to queue at %s", common.RedactURL(cfg.MQAddress))
	// the heartbeat carries name, uuid and services, so everything a client
	// needs to know about this daemon goes into its own service entry
	// TODO save uuid somewhere
	node, tr, err := common.NewNode(cfg, common.NodeConfig{
		Name:              d.fqdn,
		UUID:              uuid.NewV4().String(),
		HeartbeatInterval: heartbeatInterval,
		Logger:            cfg.Logger,
		Services: map[string]zerosvc.Service{
			common.RodrevServiceName: {
				Ok:   true,
				Info: "rodrev daemon",
				Data: common.RodrevService{
					FQDN:              d.fqdn,
					Version:           cfg.Version,
					Features:          puppet.Features,
					HeartbeatInterval: heartbeatInterval,
				},
			},
			"puppet": {Ok: true, Info: "puppet management"},
		},
	})
	if err != nil {
		return nil, err
	}
	d.node = node

	runtime := &common.Runtime{
		Node:      d.node,
		Transport: tr,
		FQDN:      d.fqdn,
		MQPrefix:  cfg.MQPrefix,
		Log:       cfg.Logger,
		Metadata:  cfg.NodeMeta,
		Cfg:       cfg,
		Debug:     cfg.Debug,
	}
	d.runtime = runtime
	d.query = query.NewQueryEngine(runtime)
	if err := d.startHeartbeatCleanup(cfg.HeartbeatCleanup); err != nil {
		// not being able to tidy up is not a reason to refuse to start
		d.l.Errorf("heartbeat cleanup not running: %s", err)
	}

	pu, err := puppet.New(puppet.Config{
		Runtime: runtime,
		Query:   d.query,
	})
	if err != nil {
		return nil, err
	}
	pu.StartServer()
	go func() {
		puppetState := mon.GlobalStatus.MustNewComponent("puppet")
		puppetState.Update(mon.StateUnknown, "initializing")
		for {
			ch, err := d.node.GetEventsCh("puppet/#")
			if err != nil {
				puppetState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
				d.l.Errorf("error connecting to channel: %s", err)
				time.Sleep(time.Second * 10)
				continue
			}
			puppetState.Update(mon.Ok, "ok")
			err = pu.EventListener(ch)
			puppetState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
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
			fencingState := mon.GlobalStatus.MustNewComponent("fencing")
			fencingState.Update(mon.StateUnknown, "initializing")
			for {
				ch, err := d.node.GetEventsCh("fence/" + d.fqdn)
				if err != nil {
					fencingState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
					d.l.Errorf("error connecting to channel: %s", err)
					time.Sleep(time.Second * 10)
					continue
				}
				fencingState.Update(mon.Ok, "ok")

				err = f.EventListener(ch)
				fencingState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
				d.l.Errorf("plugin fence exited: %s, reconnecting in 10s", err)
				time.Sleep(time.Second * 10)
			}
		}()
	}
	if len(cfg.IPSet.Sets) > 0 {
		ipsetState := mon.GlobalStatus.MustNewComponent("ipset")
		ipsetState.Update(mon.StateUnknown, "initializing")
		d.l.Infof("starting ipset management with sets [%+v]", goneric.MapSliceKey(cfg.IPSet.Sets))
		cfg.IPSet.Logger = d.l.Named("ipset")
		ipset, err := ipset.New(runtime, cfg.IPSet)
		if err != nil {
			ipsetState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
			d.l.Errorf("starting ipset failed: %s", err)
		} else {
			for _, setcfg := range cfg.IPSet.Sets {
				go func(setname config.IPSet) {
					for {
						topic := "ipset/" +
							setname.BroadcastGroup +
							"/" + setname.Name

						ch, err := d.node.GetEventsCh(topic)
						if err != nil {
							ipsetState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
							d.l.Errorf("error getting event channel for ipset [%s]: %s", err)
							time.Sleep(time.Second * 60)
							continue
						} else {
							ipsetState.Update(mon.StateOk, "ok")
							d.l.Infof("subscribing to %s", topic)
						}
						err = ipset.EventListener(ch, setname.Name)
						if err != nil {
							ipsetState.Update(mon.StateCritical, fmt.Sprintf("listen err: %s", err))
							d.l.Errorf("error on ipset [%s] event listener: %s", err)
						} else {
							ipsetState.Update(mon.StateCritical, "ipset listening exited")
						}
						time.Sleep(time.Second * 10)
					}
				}(setcfg)
			}
		}
	}
	if len(cfg.IcingaAPIURL) > 0 {
		icingaApiState := mon.GlobalStatus.MustNewComponent("icinga_api")
		icingaApiState.Update(mon.StateUnknown, "initializing")
		d.l.Infof("starting downtime plugin [%s]", cfg.IcingaAPIURL)
		api, err := downtime.NewDowntimeServer(downtime.Config{
			Icinga2URL:  cfg.IcingaAPIURL,
			Icinga2User: cfg.IcingaAPIUser,
			Icinga2Pass: cfg.IcingaAPIPass,
			Logger:      d.l,
		})
		if err != nil {
			icingaApiState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
			d.l.Errorf("error initializing icinga api: %w", err)
		} else {
			for {
				ch, err := d.node.GetEventsCh("downtime/#")
				if err != nil {
					d.l.Errorf("error initializing icinga api channel: %w", err)
					icingaApiState.Update(mon.StateCritical, fmt.Sprintf("%s", err))
					goto endapi
				}
				icingaApiState.Update(mon.StateOk, "ok")
				api.Run(ch)
				icingaApiState.Update(mon.StateCritical, "disconnected")
				d.l.Infof("restarting downtime api channel")
				time.Sleep(time.Second * 60)
			}

		}
	}
endapi:

	go func() {
		web, err := web.New(web.Config{
			Logger:       d.l,
			AccessLogger: d.l.Named("access"),
		}, embedfs.Embedded)
		if err != nil {
			d.l.Errorf("error initializing web server: %s", err)
			return
		}
		d.l.Errorf("error running webserver: %s", web.Run())
	}()
	return &d, nil
}

func (d *Daemon) exit() {
	if d.exitFunc != nil {
		d.l.Error("exiting coz of heartbeat failures")
		d.exitFunc("queue heartbeat failed")
	}
}
