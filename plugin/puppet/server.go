package puppet

import (
	"fmt"
	"github.com/efigence/rodrev/util"
	"github.com/zerosvc/go-zerosvc"
	"os"
	"strings"
)
import "time"


func (p *Puppet) StartServer() {
	go p.backgroundWorker()
}

func(p *Puppet)EventListener(evCh chan zerosvc.Event) error{
	for ev := range evCh {
		err := p.HandleEvent(&ev)
		if err != nil {
			p.l.Errorf("Error handling puppet event: %s:%+v", err,&ev)
		}
	}
	return fmt.Errorf("channel for puppet server disconnected")
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
	reqPath := strings.Split(ev.RoutingKey,"/")
	if len(reqPath) < 2 {
		return fmt.Errorf("too short path, ignoring: %s", ev.RoutingKey)
	}
	switch cmd.Command {
	case Status:
		p.lock.RLock()
		err := re.Marshal(p.lastRunSummary)
		p.lock.RUnlock()
		if err != nil {return err}
		err = ev.Reply(re)
		if err != nil {return err}
	case Run:
		// directed request e.g. puppet/host.example.com
		p.l.Warnf("path: %+v",reqPath)
		if (reqPath[len(reqPath)-1] == p.fqdn && reqPath[len(reqPath)-2]=="puppet") || // unicast
			(reqPath[len(reqPath)-1] == "puppet" && len(reqPath) == 2)  || // broadcast
			(reqPath[len(reqPath)-1] == "puppet" && reqPath[len(reqPath)-2] != "puppet") { // broadcast

			r:=p.Run()
			err := re.Marshal(&r)
			if err !=nil {
				p.l.Errorf("error marshalling: %s", err)
				return err
			}
			err = ev.Reply(re)
			if err != nil {return err}
		} else { // ignore
			p.l.Debugf("got request for path %s, ignoring as it does  not match",ev.RoutingKey, p.fqdn)
		}




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