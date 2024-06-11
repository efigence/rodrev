package fence

import (
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/plugin/fence"
	"github.com/spf13/cobra"
)

func Run(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.Help()
		return
	}
	_, runtime, log := clinit.Init(cmd)
	log.Infof("sending fence to %s from [%s]", args[0], runtime.FQDN)
	err := fence.Send(&runtime, args[0])
	if err != nil {
		log.Errorf("fence failed: %s", err)
	}
}
