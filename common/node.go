package common

import "time"

type Node struct {
	FQDN string `json:"fqdn"`
	DaemonVersion string `json:"version"`
	Services []string `json:"services,omitempty"`
	LastUpdate *time.Time `json:"last_update,omitempty"`
}
