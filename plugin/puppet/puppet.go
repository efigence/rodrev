package puppet

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/efigence/rodrev/common"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Config struct {
	LastRunSummaryYAML string `yaml:"last_run_summary"`
	LastRunReportYAML string `yaml:"last_run_report"`
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	FQDN string
	Runtime common.Runtime

}
const (
	Status = "status"
	Run = "run"
)

var DefaultConfig = Config{
	LastRunReportYAML: "/var/lib/puppet/state/last_run_report.yaml",
	LastRunSummaryYAML:  "/var/lib/puppet/state/last_run_summary.yaml",
	RefreshInterval: time.Minute,
}

type Puppet struct {
	node *zerosvc.Node
	lastRunSummary LastRunSummary
	lock sync.RWMutex
	l *zap.SugaredLogger
	cfg Config
	runLock *semaphore.Weighted
	puppetPath string
	fqdn string
	runtime common.Runtime
	rng *rand.Rand
}

func New(cfg Config) (*Puppet,error) {
	var p Puppet
	p.runLock = semaphore.NewWeighted(1)
	path, err := exec.LookPath("puppet")
	if err == nil {
		p.puppetPath = path
	} else {
		if f, err := os.Stat("/usr/local/bin/puppet"); err == nil {
			if f.Mode() & 0100 != 0 {
				p.puppetPath = "/usr/local/bin/puppet"
			}
		}
	}

	if len(cfg.LastRunSummaryYAML) == 0 { cfg.LastRunSummaryYAML = DefaultConfig.LastRunSummaryYAML }
	if len(cfg.LastRunReportYAML) == 0 { cfg.LastRunReportYAML = DefaultConfig.LastRunReportYAML }
	if cfg.RefreshInterval == 0 { cfg.RefreshInterval = DefaultConfig.RefreshInterval }
	p.l = cfg.Runtime.Log
	p.cfg = cfg
	p.node = cfg.Runtime.Node
	p.fqdn = cfg.Runtime.FQDN
	p.rng = cfg.Runtime.SeededPRNG()

	if len(p.puppetPath) == 0 {
		return nil,fmt.Errorf("can't find puppet in PATH or in /usr/local/bin")
	} else {
		p.l.Debugf("puppet path: %s",p.puppetPath)
	}

	go p.backgroundWorker()
	return &p,nil
}




type PuppetCmdSend struct {
	Command string `json:"cmd"`
	Parameters interface{} `json:"params"`
}

// wrapper so we can delay unmarshalling parameters and switch on Command
type PuppetCmdRecv struct {
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

