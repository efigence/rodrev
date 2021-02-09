package main

import (
	"github.com/efigence/rodrev/plugin/fence"
	"github.com/spf13/cobra"
)

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
	log.Infof("sending fence to %s from [%s]",args[0],runtime.FQDN)
	err := fence.SendFence(&runtime,args[0])
	if err != nil {log.Errorf("fence failed: %s",err)}
}