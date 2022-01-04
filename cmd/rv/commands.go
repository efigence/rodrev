package main

import (
	"encoding/csv"
	"encoding/json"
	"github.com/efigence/rodrev/client"
	"github.com/spf13/cobra"
	"os"
	"sort"
	"strings"
	"time"
)

func StatusRodrev(cmd *cobra.Command) error {
	cfg, runtime := Init(cmd)
	_ = cfg
	c := cmd.Flags()
	log.Info("running service discovery")
	services, nodesActive, nodesStale, err := client.Discover(&runtime)
	if err != nil {
		log.Errorf("error running discovery: %s", err)
	}
	log.Infof("services:")
	switch stringOrPanic(c.GetString("output-format")) {
	case outStderr:
		for service, nodes := range services {
			log.Infof("  %s:", service)
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].FQDN < nodes[j].FQDN })
			for _, node := range nodes {
				log.Infof("    %s:", node.FQDN)
			}
		}
		for name, node := range nodesStale {
			log.Warnf("node %s is stale: %+v", name, node)
		}

	case outCsv:
		csvW := csv.NewWriter(os.Stdout)
		csvW.Write([]string{"fqdn", "last_update", "version", "service", "active"})
		for _, info := range nodesActive {
			csvW.Write([]string{
				info.FQDN,
				info.LastUpdate.Format(time.RFC3339),
				info.DaemonVersion,
				strings.Join(info.Services, ","),
				"1",
			})
		}
		for _, info := range nodesStale {
			csvW.Write([]string{
				info.FQDN,
				info.LastUpdate.Format(time.RFC3339),
				info.DaemonVersion,
				strings.Join(info.Services, ","),
				"0",
			})
		}
		csvW.Flush()
	case outJson:
		outJ := make(map[string]interface{}, 0)
		outJ["services"] = services
		outJ["nodes_active"] = nodesActive
		outJ["nodes_stale"] = nodesStale
		err = json.NewEncoder(os.Stdout).Encode(&outJ)
		if err != nil {
			log.Errorf("error encoding node data: %s", err)
		}
	}
	return nil

}
