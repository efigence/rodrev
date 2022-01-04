package main

import (
	"github.com/efigence/rodrev/plugin/fence"
	"github.com/spf13/cobra"
	"os"
)

func FenceRun(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.Help()
		return
	}
	cfg, runtime := Init(cmd)
	_ = cfg
	c := cmd.Flags()
	_ = runtime
	_ = c
	log.Infof("sending fence to %s from [%s]", args[0], runtime.FQDN)
	err := fence.Send(&runtime, args[0])
	if err != nil {
		log.Errorf("fence failed: %s", err)
	}
}

func FenceStatus(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.Help()
		return
	}
	cfg, runtime := Init(cmd)
	_ = cfg
	c := cmd.Flags()
	_ = runtime
	_ = c
	log.Infof("sending status request to %s from [%s]", args[0], runtime.FQDN)
	ok, err := fence.Status(&runtime, args[0])
	if err != nil {
		log.Errorf("status request failed: %s", err)
		os.Exit(1)
	}
	if ok {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
