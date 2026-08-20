package puppet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/efigence/rodrev/common"
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

// wire types for the query/facts/classes commands

// QueryReply is what the Query command answers with. Every node answers,
// including the ones that did not match and the ones where the query broke,
// so the client can count them
type QueryReply struct {
	FQDN    string `json:"fqdn"`
	Matched bool   `json:"matched"`
	// Error is the query error as seen by this node, if any
	Error string `json:"error,omitempty"`
	// Type and Value describe a result that was not a boolean
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

func (q *QueryReply) RPCType() string { return common.PuppetQuery }

// FactListOptions narrows down a fact dump. A full fact set is tens of
// kilobytes per node, so asking for a few keys is worth the option
type FactListOptions struct {
	Keys []string `json:"keys,omitempty"`
}

type FactsReply struct {
	FQDN  string                 `json:"fqdn"`
	TS    time.Time              `json:"ts"`
	Count int                    `json:"count"`
	Facts map[string]interface{} `json:"facts"`
}

func (f *FactsReply) RPCType() string { return common.PuppetFacts }

type ClassesReply struct {
	FQDN    string    `json:"fqdn"`
	TS      time.Time `json:"ts"`
	Classes []string  `json:"classes"`
}

func (c *ClassesReply) RPCType() string { return common.PuppetClasses }

// Snapshot builds a snapshot out of the two dump replies. Either may be nil,
// for a node that only answered one of them
func SnapshotFromReplies(facts *FactsReply, classes *ClassesReply) *Snapshot {
	var s Snapshot
	if facts != nil {
		s.FQDN = facts.FQDN
		s.TS = facts.TS
		s.Facts = facts.Facts
	}
	if classes != nil {
		if len(s.FQDN) == 0 {
			s.FQDN = classes.FQDN
		}
		if s.TS.IsZero() {
			s.TS = classes.TS
		}
		s.Classes = classes.Classes
	}
	s.Source = "node:" + s.FQDN
	return &s
}

// Save writes the snapshot as JSON. Facts describe a host in detail, so the file
// is kept readable by its owner only
func (s *Snapshot) Save(path string) error {
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("error encoding snapshot: %w", err)
	}
	// write and rename, so an interrupted save can not leave a half file behind
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("error saving snapshot: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		return fmt.Errorf("error setting snapshot permissions: %w", err)
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return fmt.Errorf("error saving snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("error saving snapshot: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("error saving snapshot: %w", err)
	}
	return nil
}

// LoadSnapshotFile reads a snapshot saved by Save
func LoadSnapshotFile(path string) (*Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading snapshot: %w", err)
	}
	var s Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("error parsing snapshot [%s]: %w", path, err)
	}
	if len(s.Facts) == 0 && len(s.Classes) == 0 {
		return nil, fmt.Errorf("snapshot [%s] has neither facts nor classes", path)
	}
	if len(s.Source) == 0 {
		s.Source = "file:" + path
	}
	return &s, nil
}
