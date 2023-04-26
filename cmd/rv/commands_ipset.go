package main

import (
	"github.com/efigence/rodrev/plugin/ipset"
	"github.com/spf13/cobra"
)

func IpsetAdd(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		cmd.Help()
		return
	}
	cfg, runtime := Init(cmd)
	_ = cfg
	c := cmd.Flags()
	_ = runtime
	_ = c
	log.Infof("adding [%s] to ipset [%s]", args[2], args[1])
	err := ipset.Add(&runtime, args[0], args[1], args[2])
	if err != nil {
		log.Infof("ipset failed: %s", err)
	}
}
