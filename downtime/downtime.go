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

// maxDowntime is the longest downtime a host may ask for
const maxDowntime = time.Hour * 24 * 60

func (d *DowntimeServer) Run(ch chan zerosvc.Event) {
	for ev := range ch {
		downtime := DowntimeRequest{}
		err := ev.Unmarshal(&downtime)
		if err != nil {
			d.l.Warnf("wrong downtime message [%w]:%s", err, string(ev.Body))
			continue
		}
		downtime, err = ValidateRequest(downtime, ev.RoutingKey)
		if err != nil {
			d.l.Errorf("ignoring downtime request from [%s]: %s", ev.RoutingKey, err)
			continue
		}
		hosts, err := d.api.ScheduleHostDowntime(downtime.Host, icinga2.Downtime{
			Flexible:      false,
			Start:         time.Now(),
			End:           time.Now().Add(downtime.Duration),
			NoAllServices: false,
			Author:        ev.NodeName,
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

// ValidateRequest checks a downtime request against the topic it arrived on and
// fills in what the API insists on having.
//
// A host may only downtime itself, and the only trustworthy statement of who
// sent a request is the topic it was published to
func ValidateRequest(req DowntimeRequest, routingKey string) (DowntimeRequest, error) {
	if len(req.Host) < 1 {
		return req, fmt.Errorf("no hostname in request [%+v]", req)
	}
	if req.Duration <= 0 {
		return req, fmt.Errorf("downtime duration has to be positive, got %s", req.Duration)
	}
	if req.Duration >= maxDowntime {
		return req, fmt.Errorf("downtime duration %s is longer than the %s limit", req.Duration, maxDowntime)
	}
	if host := HostFromRoutingKey(routingKey); host != req.Host {
		return req, fmt.Errorf("host from route [%s] does not match requested host [%s], "+
			"a host is only allowed to downtime itself", host, req.Host)
	}
	req.Reason = strings.TrimSpace(req.Reason)
	// the api rejects an empty comment
	if len(req.Reason) == 0 {
		req.Reason = "not specified"
	}
	return req, nil
}

// HostFromRoutingKey returns the short hostname of whoever published to a topic.
// Requests arrive on <prefix>downtime/<certname>, where the certname may be a
// fqdn and may carry a client_ prefix
func HostFromRoutingKey(routingKey string) string {
	route := strings.Split(routingKey, "/")
	last := route[len(route)-1]
	return strings.TrimPrefix(strings.Split(last, ".")[0], "client_")
}
