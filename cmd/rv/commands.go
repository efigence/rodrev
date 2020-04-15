package main

import (
	"encoding/csv"
	"encoding/json"
	"github.com/efigence/rodrev/client"
	"github.com/urfave/cli"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func StatusRodrev(c *cli.Context) error {
	cfg, runtime := Init(c)
	_ = cfg
	log.Info("running service discovery")
	services, nodesActive, nodesStale, err := client.Discover(&runtime)
	if err != nil {
		log.Errorf("error running discovery: %s", err)
	}
	log.Infof("services:")
	switch c.GlobalString("output-format") {
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

func StatusPuppet(c *cli.Context) error {
	cfg, runtime := Init(c)
	_ = cfg
	status := client.PuppetStatus(&runtime,c.GlobalString("filter"))

	switch c.GlobalString("output-format") {
	case outStderr:
		log.Info("puppet status")
		for node, summary := range status {
			log.Infof("%s: %s, changes: %d/%d",
				node,
				summary.Version.Config,
				summary.Resources.Changed,
				summary.Resources.Total,
			)
		}
	case outCsv:
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
	case outJson:
		err := json.NewEncoder(os.Stdout).Encode(&status)
		if err != nil {
			log.Errorf("error encoding node data: %s", err)
		}
	}
	return nil
}
