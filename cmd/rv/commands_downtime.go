package main

import (
	"fmt"
	"github.com/efigence/rodrev/downtime"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"time"
)

func Downtime(cmd *cobra.Command, args []string) {
	if len(args) < 2 {
		cmd.Help()
		os.Exit(1)
	}
	cfg, runtime := Init(cmd)
	_ = cfg
	ev := runtime.Node.NewEvent()
	duration, err := time.ParseDuration(args[1])
	if err != nil {
		fmt.Printf("error parsing duration [%s]: %s. Example format: 30m, 23h20m. Only hms supported", args[1], err)
		os.Exit(1)
	}

	reason := ""
	if len(args) > 2 {
		reason = strings.Join(args[2:], " ")
	}
	request := downtime.DowntimeRequest{
		Host:     args[0],
		Duration: duration,
		Reason:   reason,
	}
	err = ev.Marshal(&request)
	if err != nil {
		fmt.Printf("error marshalling request: %s", err)
		os.Exit(2)
	}
	err = ev.Send("downtime/" + runtime.FQDN)
	if err != nil {
		fmt.Printf("error sending request: %s", err)
		os.Exit(2)
	}
}
