package puppet

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type RunStatus struct {
	// run was scheduled
	Scheduled bool
	// runner is busy, either running or waiting for start time
	Busy bool
	// node is in downtime
	Downtime bool
	// puppet agent started
	Started bool
	// puppet agent is applying catalog
	Applying bool
}

type RunOptions struct {
	Delay          time.Duration
	RandomizeDelay bool
}
type FactOptions struct {
	Name string
}

func (p *Puppet) Run(opt RunOptions) RunStatus {
	if !p.runLock.TryAcquire(1) {
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.runStatus
	} else {
		p.lock.Lock()
		p.runStatus.Scheduled = true
		p.lock.Unlock()
		go p.run(opt)
		p.lock.RLock()
		defer p.lock.RUnlock()
		return p.runStatus
	}
}

func (p *Puppet) run(opt RunOptions) {
	defer p.runLock.Release(1)
	var err error
	if opt.Delay > time.Hour*24 {
		p.l.Errorf("capping delay to 24 hours")
		opt.Delay = time.Hour * 24
	}
	if opt.Delay > 0 {
		if opt.RandomizeDelay {
			opt.Delay = time.Duration(p.rng.Int63n(opt.Delay.Nanoseconds()))
		}
		p.l.Infof("sleeping %ds before run", int64(opt.Delay.Seconds()))
		p.lock.Lock()
		p.runStatus.Busy = true
		p.lock.Unlock()
		time.Sleep(opt.Delay)
	}
	p.l.Info("running puppet")
	cmd := exec.Command(p.puppetPath, "agent", "--onetime", "--no-daemonize", "--verbose", "--no-splay", "--color=false")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.l.Errorf("error attaching stdin: %s", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.l.Errorf("error attaching stdin: %s", err)
		return
	}
	p.lock.Lock()
	p.runStatus.Started = true
	p.runStatus.Busy = true
	p.lock.Unlock()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sout := bufio.NewScanner(stdout)
		for sout.Scan() {
			t := sout.Text()
			p.l.Infof("+ %s", t)
			if strings.HasPrefix(t, "Info: Applying configuration version") {
				p.lock.Lock()
				p.runStatus.Applying = true
				p.lock.Unlock()

			}
		}
	}()
	go func() {
		defer wg.Done()
		serr := bufio.NewScanner(stderr)
		for serr.Scan() {
			p.l.Infof("! %s", serr.Text())
		}
	}()
	err = cmd.Start()
	if err != nil {
		p.l.Errorf("error starting puppet: %s", err)
		return
	}
	wg.Wait()
	err = cmd.Wait()
	p.lock.Lock()
	p.runStatus = RunStatus{
		Busy:     false,
		Downtime: false,
		Started:  false,
		Applying: false,
	}
	p.lock.Unlock()
	if err != nil {
		p.l.Errorf("error after puppet run: %s", err)
		return
	}
	p.l.Infof("puppet run finished")

	return

}
