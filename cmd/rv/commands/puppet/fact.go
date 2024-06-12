package puppet

import (
	"encoding/csv"
	"encoding/json"
	"github.com/efigence/rodrev/client"
	"github.com/efigence/rodrev/cmd/rv/clinit"
	"github.com/efigence/rodrev/util"
	"github.com/k0kubun/pp/v3"
	"github.com/spf13/cobra"
	"os"
)

func Fact(cmd *cobra.Command, args []string) {
	cfg, runtime, log := clinit.Init(cmd)
	c := cmd.Flags()
	_ = cfg
	if len(args) < 1 {
		log.Errorf("need at least one fact as argument")
		os.Exit(1)
	}
	status := client.PuppetFact(&runtime, args[0], util.StringOrPanic(c.GetString("filter")))

	switch util.StringOrPanic(c.GetString("output-format")) {
	case clinit.OutStderr:
		log.Info("puppet status")
		log.Info(pp.Sprint(status))
	case clinit.OutCsv:
		csvW := csv.NewWriter(os.Stdout)
		csvW.Write([]string{
			"node",
			"fact",
			"value",
		})
		for node, st := range status {
			csvW.Write([]string{
				node,
				args[0],
				pp.Sprint(st),
			})
		}
		csvW.Flush()
	case clinit.OutJson:
		err := json.NewEncoder(os.Stdout).Encode(&status)
		if err != nil {
			log.Errorf("error encoding node data: %s", err)
		}
	default:
		log.Info("unsupported output[%s]", util.StringOrPanic(c.GetString("output-format")))
		pp.Print(status)
		os.Exit(1)
	}
}
