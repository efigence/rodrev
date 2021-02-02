package fence

import "time"

const (
	FenceLocalSysrq = "local_sysrq"
	FenceRemoteLibvirt = "remote_libvirt"
)


type Config struct {
	Whitelist map[string]string
	Type string
}

var DefaultConfig = Config {
   Type: FenceLocalSysrq,
}

type FenceModule interface {
	// fences self after a delay
	// initError is "the fence method doesn't appear to work"
	// it should be returned after any pre-flight checks are done
	// runError is "I tried to fence and failed"
	// run error should return `nil` after delay or error if the fencing failed
	Self(delay time.Duration) (initError error, runError chan error)
	// same as Self but targets different node
	Node(nodeName string, delay time.Duration)  (initError error, runError chan error)
}

type Fence struct {
	cfg *Config
	fenceModule FenceModule
}




func New(cfg Config) (*Fence, error) {
	var f Fence
	f.cfg = &cfg

}
