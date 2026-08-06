package ipset

import (
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/plugin/ipset"
	"github.com/spf13/cobra"
)

func Delete(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		cmd.Help()
		return
	}
	_, runtime, log := clinit.Init(cmd)
	log.Infof("deleting [%s] from ipset [%s]", args[2], args[1])
	err := ipset.Cmd(&runtime, ipset.IPSET_DEL, args[0], args[1], args[2])
	if err != nil {
		log.Infof("ipset failed: %s", err)
	}
}
