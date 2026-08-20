package puppet

import (
	"fmt"
	"os"
	"time"
)

// Snapshot is a node's puppet state - facts, classes and optionally the last run
// summary - detached from wherever it came from. It can be re-evaluated locally
// via Harness() without touching disk or the cluster again
type Snapshot struct {
	FQDN           string                 `json:"fqdn"`
	TS             time.Time              `json:"ts"`
	Source         string                 `json:"source"`
	Facts          map[string]interface{} `json:"facts"`
	Classes        []string               `json:"classes"`
	LastRunSummary *LastRunSummary        `json:"last_run_summary,omitempty"`
}

// Harness returns a query engine loaded with the snapshot data.
// nodeMeta ends up as `node` in queries; nil generates it out of the facts
func (s *Snapshot) Harness(nodeMeta map[string]interface{}) (*QueryHarness, error) {
	h, err := NewQueryHarnessOpts(HarnessOptions{
		Facts:    s.Facts,
		Classes:  s.Classes,
		NodeMeta: nodeMeta,
		Source:   s.Source,
	})
	if err != nil {
		return nil, err
	}
	if s.LastRunSummary != nil {
		h.LastRunSummary = *s.LastRunSummary
	}
	return h, nil
}

// LoadSnapshotFiles loads a snapshot from files. Only the fact file is required;
// classes and last run summary are loaded when a path for them is given
func LoadSnapshotFiles(factsPath, classesPath, lastRunSummaryPath string) (*Snapshot, error) {
	if len(factsPath) == 0 {
		return nil, fmt.Errorf("fact file path is required")
	}
	h, err := NewQueryHarnessOpts(HarnessOptions{
		FactsPath:          factsPath,
		ClassesPath:        classesPath,
		LastRunSummaryPath: lastRunSummaryPath,
		Source:             "files:" + factsPath,
	})
	if err != nil {
		return nil, err
	}
	s := Snapshot{
		FQDN:    h.FQDN(),
		TS:      fileTime(factsPath),
		Source:  h.Source,
		Facts:   *h.Facts.Map(),
		Classes: h.Classes.List(),
	}
	if len(lastRunSummaryPath) > 0 {
		summary := h.LastRunSummary
		s.LastRunSummary = &summary
	}
	return &s, nil
}

// fileTime returns file modification time, or current time when it can't be read
func fileTime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Now()
}
