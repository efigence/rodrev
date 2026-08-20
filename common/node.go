package common

import "time"

type Node struct {
	FQDN          string   `json:"fqdn"`
	DaemonVersion string   `json:"version"`
	Services      []string `json:"services,omitempty"`
	// Features lists the commands the daemon supports, empty on older daemons
	Features   []string   `json:"features,omitempty"`
	LastUpdate *time.Time `json:"last_update,omitempty"`
	// StaleAge is how long this node's heartbeat may be silent before it counts
	// as stale, derived from how often the node says it checks in
	StaleAge time.Duration `json:"stale_age,omitempty"`
}
