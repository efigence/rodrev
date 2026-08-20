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

// QueryHarness is a set of puppet state loaded from files or from memory, with
// query engine wired up exactly like the daemon does it. Use it to check what CLI
// queries (`rv puppet --filter ...`) would match on a node with a given set of
// facts/classes. See t-data/ for an example data set
type QueryHarness struct {
	Facts          *Facts
	Classes        *Classes
	LastRunSummary LastRunSummary
	Engine         *query.Engine
	Runtime        *common.Runtime
	// Source describes where the data came from, for display
	Source string
}

// HarnessOptions is a data set for the query harness. Facts and classes can come
// either from a file or from an already loaded map/list, everything else is optional
type HarnessOptions struct {
	FactsPath          string
	ClassesPath        string
	LastRunSummaryPath string
	// Facts/Classes are used when the matching path is empty
	Facts   map[string]interface{}
	Classes []string
	// NodeMeta ends up as `node` variable in queries. When nil it is generated
	// out of the facts (see NodeMetaFromFacts)
	NodeMeta map[string]interface{}
	// Source describes where the data came from, for display
	Source string
}

// NewQueryHarness loads facts/classes/last run summary from a directory holding
// files named the way puppet names them, and registers `fact` and `class` query
// functions on top of it.
//
// nodeMeta ends up as `node` variable in queries; when nil it is generated out of
// the loaded facts (see NodeMetaFromFacts)
func NewQueryHarness(dir string, nodeMeta map[string]interface{}) (*QueryHarness, error) {
	return NewQueryHarnessOpts(HarnessOptions{
		FactsPath:          filepath.Join(dir, HarnessFactsFile),
		ClassesPath:        filepath.Join(dir, HarnessClassfile),
		LastRunSummaryPath: filepath.Join(dir, HarnessLastRunSummaryFile),
		NodeMeta:           nodeMeta,
		Source:             "dir:" + dir,
	})
}

// NewQueryHarnessOpts builds a harness out of any combination of files and
// in-memory data. Unlike NewQueryHarness the last run summary is optional, as
// anyone passing facts/classes by hand is unlikely to have one
func NewQueryHarnessOpts(o HarnessOptions) (*QueryHarness, error) {
	var h QueryHarness
	h.Source = o.Source
	var err error
	switch {
	case len(o.FactsPath) > 0:
		h.Facts, err = LoadFacts(o.FactsPath)
		if err != nil {
			return nil, fmt.Errorf("error loading facts: %w", err)
		}
	case o.Facts != nil:
		h.Facts = NewFactsFromMap(o.Facts)
	default:
		return nil, fmt.Errorf("need either fact file path or fact data")
	}
	switch {
	case len(o.ClassesPath) > 0:
		h.Classes, err = LoadClasses(o.ClassesPath)
		if err != nil {
			return nil, fmt.Errorf("error loading classes: %w", err)
		}
	case o.Classes != nil:
		h.Classes = NewClassesFromList(o.Classes)
	default:
		// classes are optional, a query can be facts-only
		h.Classes = NewClassesFromList(nil)
	}
	if len(o.LastRunSummaryPath) > 0 {
		fd, err := os.Open(o.LastRunSummaryPath)
		if err != nil {
			return nil, fmt.Errorf("error opening last run summary: %w", err)
		}
		defer fd.Close()
		h.LastRunSummary, err = ParseLastRunSummary(fd)
		if err != nil {
			return nil, fmt.Errorf("error parsing last run summary: %w", err)
		}
	}
	nodeMeta := o.NodeMeta
	if nodeMeta == nil {
		nodeMeta = h.NodeMetaFromFacts()
	}
	if len(h.Source) == 0 {
		h.Source = o.FactsPath
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
