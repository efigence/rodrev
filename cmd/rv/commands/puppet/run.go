package puppet

import (
	"github.com/efigence/rodrev/client"
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/util"
	"github.com/spf13/cobra"
	"os"
	"time"
)

func Run(cmd *cobra.Command) {
	cfg, runtime, log := clinit.Init(cmd)
	c := cmd.Flags()
	_ = cfg
	target := util.StringOrPanic(c.GetString("node"))
	noop := util.BoolOrPanic(c.GetBool("noop"))
	if len(target) == 0 {
		target = util.StringOrPanic(c.GetString("target"))
	}
	randomDelay := util.DurationOrPanic(c.GetDuration("random-delay"))
	if len(target) == 0 {
		log.Warn("need --target parameter")
		os.Exit(1)
	}
	if util.StringOrPanic(c.GetString("node")) == "all" &&
		randomDelay == 0 &&
		len(util.StringOrPanic(c.GetString("filter"))) > 3 {
		randomDelay = time.Second
	}
	if util.StringOrPanic(c.GetString("node")) == "all" && randomDelay == 0 {
		log.Errorf("do not run all 'all' without delay, if you REALLY need to run all nodes at once set random-delay to '1s' ")
		cmd.Help()
		os.Exit(1)
	}
	filter := util.StringOrPanic(c.GetString("filter"))
	if len(filter) > 0 {
		log.Warnf("filter query: %s", filter)
	}
	client.PuppetRun(&runtime, target, filter, randomDelay, client.Opts{Noop: noop})
	log.Warnf("sending  puppet run request to %s", util.StringOrPanic(c.GetString("node")))
}
