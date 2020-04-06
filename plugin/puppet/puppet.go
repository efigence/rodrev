package puppet

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"strings"
	"sync"
	"time"
)

type Config struct {
	LastRunSummaryYAML string `yaml:"last_run_summary"`
	LastRunReportYAML string `yaml:"last_run_report"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	Node *zerosvc.Node
	Logger *zap.SugaredLogger `yaml:"-"`

}
const (
	Status = "status"
)

var DefaultConfig = Config{
	LastRunReportYAML: "/var/lib/puppet/state/last_run_report.yaml",
	LastRunSummaryYAML:  "/var/lib/puppet/state/last_run_summary.yaml",
	RefreshInterval: time.Minute,
	Logger: zap.S(),
}



type Puppet struct {
	node *zerosvc.Node
	lastRunSummary LastRunSummary
	lock sync.RWMutex
	l *zap.SugaredLogger
	cfg Config
}

func New(cfg Config) (*Puppet,error) {
	var p Puppet
	if len(cfg.LastRunSummaryYAML) == 0 { cfg.LastRunSummaryYAML = DefaultConfig.LastRunSummaryYAML }
	if len(cfg.LastRunReportYAML) == 0 { cfg.LastRunReportYAML = DefaultConfig.LastRunReportYAML }
	if cfg.RefreshInterval == 0 { cfg.RefreshInterval = DefaultConfig.RefreshInterval }
	if cfg.Logger == nil { cfg.Logger = DefaultConfig.Logger }
	p.cfg = cfg
	p.l = cfg.Logger
	if cfg.Node == nil {
		return nil, fmt.Errorf("need Node in config")
	}
	p.node = cfg.Node


	go p.backgroundWorker()
	return &p,nil
}




type PuppetCmd struct {
	Command string `json:"cmd"`
	Parameters json.RawMessage `json:"params"`
}
type Msg struct {
	Msg string `json:"msg"`
}



func (p *Puppet) puppetErr(err error, msg ...string) error  {
	if len(msg) != 0 {
		return errors.New(strings.Join(msg, " ") + err.Error())
	} else {
		return fmt.Errorf("puppet error: %s", err)
	}
}

