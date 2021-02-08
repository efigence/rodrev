package main

import "github.com/spf13/cobra"

func RunFence(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.Help()
		return
	}
	cfg, runtime := Init(cmd)
	_ = cfg
	c :=  cmd.Flags()
	_ =runtime
	_ = c
	log.Infof("would fence %+v from [%s]",args,runtime.FQDN)
}