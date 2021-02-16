package hvminfo

import (
	"github.com/zerosvc/go-zerosvc"
)

// single byte please, else it will break
const CmdInfo ="I"

type HVMInfo struct {
	FQDN string `json:"fqdn"`
}

type Facts struct {
	VmHost string `yaml:"vm_host"`
}


func(h HVMInfo) Default() HVMInfo {
	if h.FQDN == "" {
		h.FQDN = zerosvc.GetFQDN()
	}
	return h
}

