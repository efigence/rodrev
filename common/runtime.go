package common

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/efigence/rodrev/config"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"log"
	mathrand "math/rand"
	"strings"
)

type Runtime struct {
	Node *zerosvc.Node
	// Transport is the connection the node runs on. Heartbeats are plain JSON
	// rather than events, so reading those goes around the node
	Transport zerosvc.Transport
	FQDN      string
	Certname  string
	// MQPrefix is the topic root. The node prefixes event paths with it on its
	// own, so it is only needed for raw subscriptions
	MQPrefix string
	Cfg      config.Config
	Metadata map[string]interface{}
	Log      *zap.SugaredLogger
	Debug    bool
}

// GetReplyChan returns a channel for replies plus the path to put in an event's
// ReplyTo. Everything below the path is subscribed as well, so one channel can
// serve many requests told apart by the topic they arrived on
func (r *Runtime) GetReplyChan() (path string, replyCh chan zerosvc.Event, err error) {
	return r.Node.GetReplyChan()
}

// SubscribeRaw subscribes to a topic under the MQ prefix and returns the
// messages as they come, undecoded. Needed for heartbeats, which are not events
func (r *Runtime) SubscribeRaw(topic string) (chan *zerosvc.Message, error) {
	if r.Transport == nil {
		return nil, fmt.Errorf("no transport, this runtime can not subscribe")
	}
	// buffered: the transport hands messages over from its own reader, and a
	// subscription that blocks stalls the whole client
	ch := make(chan *zerosvc.Message, 256)
	err := r.Transport.Subscribe(EventRoot(r.MQPrefix)+"/"+topic, ch)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// Reply answers an event. It keeps the correlation id, so a client running
// several requests over one reply channel can tell the answers apart
func (r *Runtime) Reply(ev *zerosvc.Event, reply zerosvc.Event) error {
	if len(ev.ReplyTo) == 0 {
		return fmt.Errorf("no reply-to in event, can not answer it")
	}
	if id, ok := ev.Headers["correlation-id"]; ok {
		reply.Headers["correlation-id"] = id
	}
	return r.Node.SendEvent(ev.ReplyTo, reply)
}

func (r *Runtime) RngBlob(bytes int) []byte {
	rnd := make([]byte, bytes)
	i, err := rand.Read(rnd)
	if err == nil && i == bytes {
		return rnd
	}
	var errctr uint8
	var readctr = i
	for {
		errctr++
		if errctr > 10 {
			log.Panicf("could not get data from RNG")
		}
		i, err := rand.Read(rnd[readctr:])
		if i > 0 {
			readctr += i
		} else {
			r.Log.Errorf("error getting RNG: %s", err)
		}
		if readctr >= bytes {
			return rnd
		}
	}
}

func (r *Runtime) SeededPRNG() *mathrand.Rand {
	blob := r.RngBlob(8)
	seed := binary.BigEndian.Uint64(blob)
	return mathrand.New(mathrand.NewSource(int64(seed)))
}
func (r *Runtime) UnlikelyErr(err error, extra ...string) {
	if err != nil {
		if len(extra) == 0 {
			r.Log.Errorf("error: %s", err)
		} else {
			r.Log.Errorf("%+v", extra, err)
		}
	}
}

var base64Replacer = strings.NewReplacer(
	"+", "_",
	"/", "-",
)

// MapBytesToTopicTitle maps binary data to topic-friendly subset of characters.
func MapBytesToTopicTitle(data []byte) string {
	str := base64.StdEncoding.EncodeToString(data)
	return base64Replacer.Replace(strings.Trim(str, "="))
}
