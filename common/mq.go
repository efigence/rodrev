package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/efigence/rodrev/config"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
)

// RodrevService is the data rvd publishes about itself, as the Data field of the
// "rodrev" entry of its heartbeat services. The heartbeat itself only carries
// name, uuid and services, so anything else a client needs to know goes here
type RodrevService struct {
	FQDN     string `json:"fqdn"`
	Version  string `json:"version,omitempty"`
	Certname string `json:"certname,omitempty"`
	// Features lists the commands the daemon understands
	Features []string `json:"features,omitempty"`
	// HeartbeatInterval is how often this node checks in, so a client can tell
	// a heartbeat that is merely old from one that is stale
	HeartbeatInterval time.Duration `json:"heartbeat_interval,omitempty"`
}

// RodrevServiceName is the service entry rvd announces itself under
const RodrevServiceName = "rodrev"

// ParseRodrevService pulls rvd's own data out of a heartbeat service entry.
// Service.Data arrives as a generic map, so it takes a round trip through JSON
func ParseRodrevService(svc zerosvc.Service) (RodrevService, bool) {
	var out RodrevService
	if svc.Data == nil {
		return out, false
	}
	raw, err := json.Marshal(svc.Data)
	if err != nil {
		return out, false
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, false
	}
	return out, len(out.FQDN) > 0
}

// NodeConfig is what a client or the daemon needs to show up on the MQ
type NodeConfig struct {
	// Name identifies the node in every event it sends
	Name string
	UUID string
	// ID is the MQTT client id. Defaults to Name
	ID string
	// HeartbeatInterval is how often to announce ourselves
	HeartbeatInterval time.Duration
	// Services is announced in the heartbeat
	Services map[string]zerosvc.Service
	Logger   *zap.SugaredLogger
}

// NewNode connects to the MQ and returns a node to send events with, plus the
// transport it runs on - heartbeats are plain JSON rather than events, so
// reading those needs the transport itself
func NewNode(cfg config.Config, nc NodeConfig) (*zerosvc.Node, zerosvc.Transport, error) {
	if len(nc.Name) == 0 {
		return nil, nil, fmt.Errorf("node name required")
	}
	if len(nc.ID) == 0 {
		nc.ID = nc.Name
	}
	mqURL, err := TransportURL(cfg.MQAddress)
	if err != nil {
		return nil, nil, err
	}
	tr, err := zerosvc.NewTransportMQTTv3(zerosvc.ConfigMQTTv3{
		ID:      nc.ID,
		MQTTURL: []*url.URL{mqURL},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error setting up transport for %s: %s", RedactURL(cfg.MQAddress), err)
	}
	node, err := zerosvc.NewNode(zerosvc.Config{
		NodeName:          nc.Name,
		NodeUUID:          nc.UUID,
		Transport:         tr,
		EventRoot:         EventRoot(cfg.MQPrefix),
		HeartbeatInterval: nc.HeartbeatInterval,
		Logger:            nc.Logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("can't connect to queue at %s: %s", RedactURL(cfg.MQAddress), err)
	}
	// the node starts announcing itself the moment it is built, so its service
	// map is already being read by that goroutine while we fill it in
	node.Lock()
	for name, svc := range nc.Services {
		node.Services[name] = svc
	}
	node.Unlock()
	return node, tr, nil
}

// TransportURL parses an MQ address into what the transport expects.
//
// The scheme is normalized to ssl:// for TLS, because that is the one the
// transport loads client certificates and CAs for, while rodrev configs have
// always used tls://
func TransportURL(address string) (*url.URL, error) {
	if len(address) == 0 {
		return nil, fmt.Errorf("no MQ address configured")
	}
	u, err := url.Parse(address)
	if err != nil {
		// the parse error carries the whole url, password included
		return nil, fmt.Errorf("can't parse MQ url [%s]: %s", RedactURL(address), causeOf(err))
	}
	if u.Scheme == "tls" {
		u.Scheme = "ssl"
	}
	return u, nil
}

// causeOf strips the wrapper off a parse error. url.Parse puts the url it choked
// on into the message, and an MQ url has credentials in it
func causeOf(err error) error {
	if cause := errors.Unwrap(err); cause != nil {
		return cause
	}
	return err
}

// EventRoot turns the configured MQ prefix into the topic root the node uses.
// The node adds the separator itself, so a trailing slash has to go
func EventRoot(prefix string) string {
	return strings.TrimSuffix(prefix, "/")
}
