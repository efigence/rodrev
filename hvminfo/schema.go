package hvminfo

import (
	"github.com/zerosvc/go-zerosvc"
)

type HVMInfo struct {
	FQDN string `json:"fqdn"`
}

func(h HVMInfo) Default() HVMInfo {
	if h.FQDN == "" {
		h.FQDN = zerosvc.GetFQDN()
	}
	return h
}

