package puppet

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
)

type LastRunSummary struct {
	Version LastRunVersion `yaml:"version"`
	Timing LastRunTiming `yaml:"time"`
	Resources LastRunResources `yaml:"resources"`
	Events LastRunEvents `yaml:"events"`

}

type LastRunTiming struct {
	Duration map[string]float64 `yaml:",inline"`
	LastRun int `yaml:"last_run"`
}

type LastRunResources struct {
    Changed int `yaml:"changed"`
    CorrectiveChange int `yaml:"corrective_change"`
    Failed int `yaml:"failed"`
    FailedToTestart int `yaml:"failed_to_restart"`
    OutOfSync int `yaml:"out_of_sync"`
    Restarted int `yaml:"restarted"`
    Scheduled int `yaml:"scheduled"`
    Skipped int `yaml:"skipped"`
    Total int `yaml:"total"`
}

type LastRunEvents struct {
    Failure int `yaml:"failure"`
    Success int `yaml:"success"`
    Total int `yaml:"total"`
}
type LastRunVersion struct {
	Config string `yaml:"config"`
	Puppet string `yaml:"puppet"`
}

func ParseLastRunSummary(r io.Reader) (*LastRunSummary, error) {
	s := LastRunSummary{}
	err := yaml.NewDecoder(r).Decode(&s)
	if err != nil {
		return nil, fmt.Errorf("error parsing puppet summary: %s", err)
	}
	return &s,nil

}