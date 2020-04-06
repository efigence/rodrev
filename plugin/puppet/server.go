package puppet

import (
	"fmt"
	"github.com/efigence/rodrev/util"
	"github.com/zerosvc/go-zerosvc"
	"os"
)
import "time"


func (p *Puppet) StartServer(evCh chan zerosvc.Event) {
	go p.backgroundWorker()
	go func() {
		for ev := range evCh {
			err := p.HandleEvent(&ev)
			if err != nil {
				p.l.Errorf("Error handling puppet event: %s", err)
			}
		}
	}()
}

func (p *Puppet) HandleEvent(ev *zerosvc.Event) error {
	var cmd PuppetCmd
	err := ev.Unmarshal(&cmd)
	if err!=nil {
		return p.puppetErr(err)
	}
	if len(ev.ReplyTo) == 0 {
		return fmt.Errorf("no reply-to in incoming event, aborting: %+v", ev)
	}
	re := p.node.NewEvent()
	re.Headers["fqdn"]=util.GetFQDN()
	switch cmd.Command {
	case Status:
		p.lock.RLock()
		err := re.Marshal(p.lastRunSummary)
		p.lock.RUnlock()
		if err != nil {return err}
		err = ev.Reply(re)
		if err != nil {return err}
	default:
		re := p.node.NewEvent()
		re.Marshal(&Msg{Msg: "unknown command " + cmd.Command})
		ev.Reply(re)
		p.l.Warnf("unkown command %s",cmd.Command)


	}
	return nil
}



func (p *Puppet) backgroundWorker() {
	for {
		p.updateLastRunSummary()
		time.Sleep(p.cfg.RefreshInterval)

	}

}

func (p *Puppet) updateLastRunSummary() {
	fd, err := os.Open(p.cfg.LastRunSummaryYAML)
	if err != nil {
		p.l.Errorf("could not open puppet run summary [%s]: %s", p.cfg.LastRunSummaryYAML, err)
		return
	}
	summary, err := ParseLastRunSummary(fd)
	if err != nil {
		p.l.Warnf("error parsing last run summary: %s")
		return
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	p.lastRunSummary = summary
}