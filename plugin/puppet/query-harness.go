package puppet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/efigence/rodrev/query"
	"go.uber.org/zap"
)

// file names within a query harness data directory. Same names puppet uses
const (
	HarnessFactsFile          = "facts.yaml"
	HarnessClassfile          = "classes.txt"
	HarnessLastRunSummaryFile = "last_run_summary.yaml"
)

// QueryHarness is a set of puppet state loaded from a directory, with query engine
// wired up exactly like the daemon does it. Use it to check what CLI queries
// (`rv puppet --filter ...`) would match on a node with a given set of
// facts/classes. See t-data/ for an example data set
type QueryHarness struct {
	Facts          *Facts
	Classes        *Classes
	LastRunSummary LastRunSummary
	Engine         *query.Engine
	Runtime        *common.Runtime
	dir            string
}

// NewQueryHarness loads facts/classes/last run summary from dir and registers
// `fact` and `class` query functions on top of it.
//
// nodeMeta ends up as `node` variable in queries; when nil it is generated out of
// the loaded facts (see NodeMetaFromFacts)
func NewQueryHarness(dir string, nodeMeta map[string]interface{}) (*QueryHarness, error) {
	var h QueryHarness
	h.dir = dir
	var err error
	h.Facts, err = LoadFacts(filepath.Join(dir, HarnessFactsFile))
	if err != nil {
		return nil, fmt.Errorf("error loading facts: %w", err)
	}
	h.Classes, err = LoadClasses(filepath.Join(dir, HarnessClassfile))
	if err != nil {
		return nil, fmt.Errorf("error loading classes: %w", err)
	}
	fd, err := os.Open(filepath.Join(dir, HarnessLastRunSummaryFile))
	if err != nil {
		return nil, fmt.Errorf("error opening last run summary: %w", err)
	}
	defer fd.Close()
	h.LastRunSummary, err = ParseLastRunSummary(fd)
	if err != nil {
		return nil, fmt.Errorf("error parsing last run summary: %w", err)
	}
	if nodeMeta == nil {
		nodeMeta = h.NodeMetaFromFacts()
	}
	fqdn, _ := nodeMeta["fqdn"].(string)
	h.Runtime = &common.Runtime{
		FQDN:     fqdn,
		Certname: fqdn,
		MQPrefix: "rv/",
		Cfg: config.Config{
			NodeMeta: nodeMeta,
		},
		Log: zap.NewNop().Sugar(),
	}
	h.Engine = query.NewQueryEngine(h.Runtime)
	if err := h.Engine.RegisterMap("fact", h.Facts); err != nil {
		return nil, fmt.Errorf("error registering facts in query engine: %w", err)
	}
	if err := h.Engine.RegisterMap("class", h.Classes); err != nil {
		return nil, fmt.Errorf("error registering classes in query engine: %w", err)
	}
	return &h, nil
}

// Query runs a CLI-style filter query against loaded data
func (h *QueryHarness) Query(q string) (bool, error) {
	return h.Engine.ParseBool(q)
}

// FQDN returns node fqdn out of loaded facts
func (h *QueryHarness) FQDN() string {
	facts := *h.Facts.Map()
	if v, ok := facts["fqdn"].(string); ok && len(v) > 0 {
		return v
	}
	// newer facter only has it under networking
	if networking, ok := facts["networking"].(map[string]interface{}); ok {
		if v, ok := networking["fqdn"].(string); ok {
			return v
		}
	}
	return ""
}

// NodeMetaFromFacts generates node metadata (config's node_meta, `node` variable in
// queries) out of loaded facts
func (h *QueryHarness) NodeMetaFromFacts() map[string]interface{} {
	facts := *h.Facts.Map()
	fqdn := h.FQDN()
	meta := map[string]interface{}{
		"fqdn":     fqdn,
		"certname": fqdn,
	}
	for _, key := range []string{"site", "mq_group", "project", "accounting_project"} {
		if v, ok := facts[key]; ok {
			meta[key] = v
		}
	}
	return meta
}
