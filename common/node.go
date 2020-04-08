package common

import "time"

type Node struct {
	FQDN string
	LastUpdate time.Time
}
