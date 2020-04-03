package puppet

import "time"

type Config struct {
	LastRunSummaryYAML string `yaml:"last_run_summary"`
	LastRunReportYAML string `yaml:"last_run_report"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`

}

var DefaultConfig = Config{
	LastRunReportYAML: "/var/lib/puppet/state/last_run_report.yaml",
	LastRunSummaryYAML:  "/var/lib/puppet/state/last_run_summary.yaml",
	RefreshInterval: time.Minute,
}


type Puppet struct {
}

func New(cfg Config) (*Puppet,error) {
	var p Puppet


	return &p,nil
}

