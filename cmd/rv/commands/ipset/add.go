package ipset

import (
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/plugin/ipset"
	"github.com/spf13/cobra"
)

func Add(cmd *cobra.Command, args []string) {
	if len(args) < 3 {
		cmd.Help()
		return
	}
	_, runtime, log := clinit.Init(cmd)
	log.Infof("adding [%s] to ipset [%s]", args[2], args[1])
	err := ipset.Add(&runtime, args[0], args[1], args[2])
	if err != nil {
		log.Infof("ipset failed: %s", err)
	}
}
