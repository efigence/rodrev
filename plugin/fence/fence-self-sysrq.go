package fence

import (
	"fmt"
	"time"
	"github.com/efigence/rodrev/sysrq"
)

type fenceSelf struct {
}



func (f *fenceSelf) Self(delay time.Duration) (initError error, runError chan error) {
	runCh := make(chan error, 1)
	// TODO run sysrq test
	go func() {
		if delay > 0 {
			time.Sleep(delay)
			runCh <- sysrq.Trigger(sysrq.CmdSync)
		}
	}()
	return nil, runCh
}
func (f *fenceSelf) Node(nodeName string, delay time.Duration)  (initError error, runError chan error) {
	return fmt.Errorf("sysrq fence works only on self"),make(chan error,1)
}
