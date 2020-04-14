package puppet

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
)

type LastRunSummary struct {
	Version   LastRunVersion   `yaml:"version" json:"version"`
	Timing    LastRunTiming    `yaml:"time" json:"timing"`
	Resources LastRunResources `yaml:"resources" json:"resources"`
	Events    LastRunEvents    `yaml:"events" json:"events"`
}

type LastRunTiming struct {
	Duration map[string]float64 `yaml:",inline" json:"duration"`
	LastRun  int                `yaml:"last_run" json:"last_run_ts"`
}

type LastRunResources struct {
	Changed          int `yaml:"changed" json:"changed"`
	CorrectiveChange int `yaml:"corrective_change" json:"corrective_change"`
	Failed           int `yaml:"failed" json:"failed"`
	FailedToTestart  int `yaml:"failed_to_restart" json:"failed_to_restart"`
	OutOfSync        int `yaml:"out_of_sync" json:"out_of_sync"`
	Restarted        int `yaml:"restarted" json:"restarted"`
	Scheduled        int `yaml:"scheduled" json:"scheduled"`
	Skipped          int `yaml:"skipped" json:"skipped"`
	Total            int `yaml:"total" json:"total"`
}

type LastRunEvents struct {
	Failure int `yaml:"failure" json:"failure"`
	Success int `yaml:"success" json:"success"`
	Total   int `yaml:"total" json:"total"`
}
type LastRunVersion struct {
	Config string `yaml:"config" json:"config"`
	Puppet string `yaml:"puppet" json:"puppet"`
}

func ParseLastRunSummary(r io.Reader) (LastRunSummary, error) {
	s := LastRunSummary{}
	err := yaml.NewDecoder(r).Decode(&s)
	if err != nil {
		return LastRunSummary{}, fmt.Errorf("error parsing puppet summary: %s", err)
	}
	return s, nil

}
