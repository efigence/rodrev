package puppet

import (
	"encoding/csv"
	"encoding/json"
	"github.com/efigence/rodrev/client"
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/util"
	"github.com/spf13/cobra"
	"os"
	"strconv"
	"time"
)

func Status(cmd *cobra.Command) {
	cfg, runtime, log := clinit.Init(cmd)
	c := cmd.Flags()
	_ = cfg
	status := client.PuppetStatus(&runtime, util.StringOrPanic(c.GetString("filter")))

	switch util.StringOrPanic(c.GetString("output-format")) {
	case clinit.OutStderr:
		log.Info("puppet status")
		for node, summary := range status {
			log.Infof("%s: %s, changes: %d/%d",
				node,
				summary.Version.Config,
				summary.Resources.Changed,
				summary.Resources.Total,
			)
		}
	case clinit.OutCsv:
		csvW := csv.NewWriter(os.Stdout)
		csvW.Write([]string{
			"fqdn",
			"last_run",
			"changed",
			"total",
			"duration",
			"puppet_version",
			"config_version",
		})
		for node, summary := range status {
			totalDuration := "0"
			if v, ok := summary.Timing.Duration["total"]; ok {
				totalDuration = strconv.FormatFloat(v, 'f', 2, 64)
			}
			csvW.Write([]string{
				node,
				time.Unix(int64(summary.Timing.LastRun), 0).Format(time.RFC3339),
				strconv.Itoa(summary.Resources.Changed),
				strconv.Itoa(summary.Resources.Total),
				totalDuration,
				summary.Version.Puppet,
				summary.Version.Config,
			})
		}
		csvW.Flush()
	case clinit.OutJson:
		err := json.NewEncoder(os.Stdout).Encode(&status)
		if err != nil {
			log.Errorf("error encoding node data: %s", err)
		}
	}
}
