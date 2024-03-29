package downtime

import (
	"fmt"
	"github.com/efigence/go-icinga2"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"strings"
	"time"
)

type DowntimeServer struct {
	l   *zap.SugaredLogger
	api *icinga2.API
}
type Config struct {
	Icinga2URL  string
	Icinga2User string
	Icinga2Pass string
	Logger      *zap.SugaredLogger
}

type DowntimeRequest struct {
	Host     string
	Duration time.Duration
	Reason   string
}

func NewDowntimeServer(cfg Config) (*DowntimeServer, error) {
	api, err := icinga2.New(cfg.Icinga2URL, cfg.Icinga2User, cfg.Icinga2Pass)
	s := &DowntimeServer{
		l: cfg.Logger,
	}
	if err != nil {
		return nil, fmt.Errorf("icinga API error: %w", err)
	}
	if s.l == nil {
		return nil, fmt.Errorf("pass logger")
	}
	if len(cfg.Icinga2URL) < 5 {
		return nil, fmt.Errorf("pass icinga url")
	}
	hosts, err := api.GetHosts()
	if err != nil {
		return nil, fmt.Errorf("couldn't connect to [%s]: %w", cfg.Icinga2URL, err)
	}
	s.api = api
	s.l.Infof("icinga2 api started, found [%d] hosts", len(hosts))
	return s, nil
}
func (d *DowntimeServer) Run(ch chan zerosvc.Event) {
	for ev := range ch {
		downtime := DowntimeRequest{}
		err := ev.Unmarshal(&downtime)
		if err != nil {
			d.l.Warnf("wrong downtime message [%w]:%s", err, string(ev.Body))
			continue
		}
		if len(downtime.Host) < 1 || downtime.Duration <= 0 || downtime.Duration >= time.Hour*24*60 {
			d.l.Warnf("need hostname and duration shorter than 2 months [%+v]", downtime)
			continue
		}
		hostFromRoute := ""
		route := strings.Split(ev.RoutingKey, "/")
		if len(route) > 0 {
			h := strings.Split(route[len(route)-1], ".")
			if len(h) > 0 {
				hostFromRoute = strings.TrimPrefix(h[0], "client_")

			}

		}
		if hostFromRoute != downtime.Host {
			d.l.Errorf("host from route [%s/%s] does not match requested host[%s]. Host is only allowed to downtime itself", ev.RoutingKey, hostFromRoute, downtime.Host)
			continue
		}
		downtime.Reason = strings.TrimSpace(downtime.Reason)
		// downtime reason can't be empty, else api fails
		if len(downtime.Reason) == 0 {
			downtime.Reason = "not specified"
		}
		hosts, err := d.api.ScheduleHostDowntime(downtime.Host, icinga2.Downtime{
			Flexible:      false,
			Start:         time.Now(),
			End:           time.Now().Add(downtime.Duration),
			NoAllServices: false,
			Author:        ev.NodeName(),
			Comment:       downtime.Reason,
		})
		if err != nil {
			d.l.Warnf("error downtiming %s: %w", downtime.Host, err)
		} else if len(hosts) == 0 {
			d.l.Warnf("no host matching %s for downtime", downtime.Host)
		} else {
			d.l.Infof("downtimed [%+v]", hosts)
		}
	}
}
