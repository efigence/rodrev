package downtime

import (
	"fmt"
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/downtime"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"time"
)

func Downtime(cmd *cobra.Command, args []string) {
	cfg, runtime, log := clinit.Init(cmd)
	if len(args) < 1 {
		cmd.Help()
		os.Exit(1)
	}
	c := cmd.Flags()
	hostRaw, err := c.GetString("host")
	if err != nil {
		fmt.Printf("could not get host flag: %s", err)
	}
	if len(hostRaw) == 0 {
		hostRaw, _ = os.Hostname()
	}
	durationRaw := args[0]

	ev := runtime.Node.NewEvent()
	duration, err := time.ParseDuration(durationRaw)
	if err != nil {
		log.Warnf("error parsing duration [%s]: %s. Example format: 30m, 23h20m. Only hms supported", args[1], err)
		os.Exit(1)
	}

	reason := ""
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}
	request := downtime.DowntimeRequest{
		Host:     hostRaw,
		Duration: duration,
		Reason:   reason,
	}
	err = ev.Marshal(&request)
	if err != nil {
		log.Warnf("error marshalling request: %s\n", err)
		os.Exit(2)
	}
	err = ev.Send(cfg.MQPrefix + "downtime/" + runtime.Certname)
	if err != nil {
		fmt.Printf("error sending request: %s\n", err)
		os.Exit(2)
	}
}
