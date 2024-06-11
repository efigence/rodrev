package fence

import (
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/plugin/fence"
	"github.com/spf13/cobra"
	"os"
)

func Status(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.Help()
		return
	}
	_, runtime, log := clinit.Init(cmd)
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
