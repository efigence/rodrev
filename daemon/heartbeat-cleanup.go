package daemon

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
)

const (
	// discoveryTopicFilter is where nodes announce themselves
	discoveryTopicFilter = "discovery/#"
	// legacyHeartbeatFilter is where they used to. Nothing publishes there any
	// more, so whatever is retained on it is left over
	legacyHeartbeatFilter = "heartbeat/#"

	defaultCleanupMaxAge       = time.Hour * 24 * 30
	defaultCleanupInterval     = time.Hour * 24 * 7
	defaultCleanupInitialDelay = time.Minute * 5
	// initialSpread/intervalSpread randomize the schedule so a fleet does not
	// all do this in the same moment
	initialSpread  = 0.5
	intervalSpread = 0.2
)

// cleanupPublisher removes a retained message
type cleanupPublisher interface {
	SendCleanup(path string) error
}

// heartbeatCache is the retained presence as it currently stands on the broker.
//
// It has to be followed rather than polled: retained messages are only delivered
// when a subscription is made, and subscribing to the same filter again does not
// bring them back, so a weekly pass could not go and ask for them
type heartbeatCache struct {
	l       sync.Mutex
	entries map[string][]byte
}

func newHeartbeatCache() *heartbeatCache {
	return &heartbeatCache{entries: make(map[string][]byte, 64)}
}

// follow keeps the cache in step with what is on the broker
func (c *heartbeatCache) follow(ch chan *zerosvc.Message) {
	for msg := range ch {
		c.l.Lock()
		if len(msg.Payload) == 0 {
			// the retained message was removed, by a node's will or by us
			delete(c.entries, msg.Topic)
		} else {
			payload := make([]byte, len(msg.Payload))
			copy(payload, msg.Payload)
			c.entries[msg.Topic] = payload
		}
		c.l.Unlock()
	}
}

func (c *heartbeatCache) snapshot() map[string][]byte {
	c.l.Lock()
	defer c.l.Unlock()
	out := make(map[string][]byte, len(c.entries))
	for topic, payload := range c.entries {
		out[topic] = payload
	}
	return out
}

func (c *heartbeatCache) forget(topic string) {
	c.l.Lock()
	defer c.l.Unlock()
	delete(c.entries, topic)
}

// cleanupOpts is one cleanup pass' worth of settings
type cleanupOpts struct {
	MaxAge time.Duration
	DryRun bool
	// EventRoot is stripped off a topic before it is handed back to the node
	EventRoot string
	// Keep is our own presence topic, which is never removed
	Keep string
}

// startHeartbeatCleanup subscribes to the presence topics and schedules the
// cleanup: first pass a few minutes in, then roughly weekly, both randomized
func (d *Daemon) startHeartbeatCleanup(cfg config.HeartbeatCleanup) error {
	if cfg.Disabled {
		d.l.Debugf("heartbeat cleanup disabled")
		return nil
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = defaultCleanupMaxAge
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultCleanupInterval
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = defaultCleanupInitialDelay
	}
	cache := newHeartbeatCache()
	for _, filter := range []string{discoveryTopicFilter, legacyHeartbeatFilter} {
		ch, err := d.runtime.SubscribeRaw(filter)
		if err != nil {
			return fmt.Errorf("can't watch presence topic %s: %s", filter, err)
		}
		go cache.follow(ch)
	}
	go d.heartbeatCleanupLoop(cache, cfg)
	return nil
}

func (d *Daemon) heartbeatCleanupLoop(cache *heartbeatCache, cfg config.HeartbeatCleanup) {
	rng := d.runtime.SeededPRNG()
	opts := cleanupOpts{
		MaxAge:    cfg.MaxAge,
		DryRun:    cfg.DryRun,
		EventRoot: common.EventRoot(d.prefix),
		Keep:      d.presenceTopic(),
	}
	// the first pass waits a few minutes: a node that is merely restarting
	// should have checked in again by then
	delay := jittered(rng, cfg.InitialDelay, initialSpread)
	for {
		d.l.Debugf("next heartbeat cleanup in %s", delay.Round(time.Second))
		time.Sleep(delay)
		cleared, seen := runHeartbeatCleanup(cache, d.node, opts, d.l)
		if cleared > 0 || d.runtime.Debug {
			d.l.Infof("heartbeat cleanup: %d of %d retained presence messages removed%s",
				cleared, seen, dryRunNote(opts.DryRun))
		}
		delay = jittered(rng, cfg.Interval, intervalSpread)
	}
}

// presenceTopic is where this node's own heartbeat lives
func (d *Daemon) presenceTopic() string {
	return strings.Join([]string{
		common.EventRoot(d.prefix), "discovery", d.node.Name, d.node.UUID,
	}, "/")
}

// runHeartbeatCleanup removes the retained presence of nodes that are gone
func runHeartbeatCleanup(cache *heartbeatCache, pub cleanupPublisher, opts cleanupOpts,
	l *zap.SugaredLogger) (cleared int, seen int) {
	now := time.Now()
	entries := cache.snapshot()
	for topic, payload := range entries {
		seen++
		if topic == opts.Keep {
			continue
		}
		reason, stale := staleHeartbeat(topic, payload, now, opts.MaxAge)
		if !stale {
			continue
		}
		if opts.DryRun {
			l.Infof("would remove retained presence [%s]: %s", topic, reason)
			cleared++
			continue
		}
		path := relativeTopic(topic, opts.EventRoot)
		if err := pub.SendCleanup(path); err != nil {
			l.Warnf("could not remove retained presence [%s]: %s", topic, err)
			continue
		}
		l.Infof("removed retained presence [%s]: %s", topic, reason)
		cache.forget(topic)
		cleared++
	}
	return cleared, seen
}

// staleHeartbeat decides whether a retained presence message is worth keeping,
// and says why when it is not
func staleHeartbeat(topic string, payload []byte, now time.Time, maxAge time.Duration) (string, bool) {
	if len(payload) == 0 {
		// already gone
		return "", false
	}
	if strings.Contains(topic, "/heartbeat/") {
		return "left over from the heartbeat topic older daemons used", true
	}
	var info zerosvc.NodeInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return fmt.Sprintf("presence data can not be parsed (%d bytes): %s", len(payload), err), true
	}
	if info.TS.IsZero() {
		return "presence data has no timestamp", true
	}
	if age := now.Sub(info.TS); age > maxAge {
		return fmt.Sprintf("last seen %s ago", age.Round(time.Hour)), true
	}
	return "", false
}

// relativeTopic strips the event root, which the node adds back on publish
func relativeTopic(topic string, eventRoot string) string {
	if len(eventRoot) == 0 {
		return topic
	}
	return strings.TrimPrefix(topic, eventRoot+"/")
}

// jittered spreads a delay by up to spread in both directions, so a fleet does
// not converge on doing the same thing at the same time
func jittered(rng *rand.Rand, d time.Duration, spread float64) time.Duration {
	if d <= 0 {
		return d
	}
	if spread <= 0 {
		return d
	}
	// rng.Float64() is [0,1), so this lands in [d-spread*d, d+spread*d)
	offset := (rng.Float64()*2 - 1) * spread * float64(d)
	out := time.Duration(float64(d) + offset)
	if out <= 0 {
		return time.Second
	}
	return out
}

func dryRunNote(dryRun bool) string {
	if dryRun {
		return " (dry run, nothing was touched)"
	}
	return ""
}
