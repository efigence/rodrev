package client

import (
	"os"
	"testing"
	"time"
)

// the production defaults, captured before the tests shorten them
var (
	productionQueryTimeout  = DefaultQueryTimeout
	productionDiscoverOpts  = DefaultDiscoverOpts
	testQueryTimeout        = time.Millisecond * 200
	testDiscoverInitialWait = time.Millisecond * 500
	testDiscoverIdleWait    = time.Millisecond * 100
)

// TestMain shortens the waits the package is built around. Every call runs until
// its deadline, so with the real ones this suite would spend most of its time
// waiting for replies that already arrived
func TestMain(m *testing.M) {
	DefaultQueryTimeout = testQueryTimeout
	DefaultDiscoverOpts = DiscoverOpts{
		InitialWait: testDiscoverInitialWait,
		IdleWait:    testDiscoverIdleWait,
	}
	os.Exit(m.Run())
}

// the shortened waits above must not hide a change to what ships
func TestPackageDefaults(t *testing.T) {
	if productionQueryTimeout != time.Second*4 {
		t.Errorf("default query timeout is %s, expected 4s", productionQueryTimeout)
	}
	if productionDiscoverOpts.InitialWait != time.Second*10 {
		t.Errorf("default discovery initial wait is %s, expected 10s", productionDiscoverOpts.InitialWait)
	}
	if productionDiscoverOpts.IdleWait != time.Second*4 {
		t.Errorf("default discovery idle wait is %s, expected 4s", productionDiscoverOpts.IdleWait)
	}
}
