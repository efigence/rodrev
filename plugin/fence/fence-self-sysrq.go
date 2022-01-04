package fence

import (
	"fmt"
	"github.com/efigence/rodrev/sysrq"
	"os"
	"os/signal"
	"time"
)

type fenceSelf struct {
}

func (f *fenceSelf) Self(delay time.Duration) (initError error, runError chan error) {
	runCh := make(chan error, 1)
	// TODO run sysrq test
	go func() {
		// make ourselves signal-proof
		c := make(chan os.Signal, 16)
		signal.Notify(c)
		go func() {
			for sig := range c {
				fmt.Printf("got signal %s", sig)
			}
		}()
		sysrq.Trigger(sysrq.CmdSync)
		sysrq.Trigger(sysrq.CmdTerm)
		time.Sleep(delay)
		runCh <- sysrq.Trigger(sysrq.CmdReadonly) // point where client gets confirmation
		time.Sleep(time.Second * 20)
		sysrq.Trigger(sysrq.CmdReboot)
	}()
	return nil, runCh
}
func (f *fenceSelf) Node(nodeName string, delay time.Duration) (initError error, runError chan error) {
	return fmt.Errorf("sysrq fence works only on self"), make(chan error, 1)
}
