package common

import (
	mathrand "math/rand"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"github.com/efigence/rodrev/util"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
	"log"
	"strings"
)

type Runtime struct {
	Node *zerosvc.Node
	FQDN string
	MQPrefix string
	Log *zap.SugaredLogger
}

// GetReplyChan() returns randomly generated channel for replies
func (r *Runtime)GetReplyChan() (path string, replyCh chan zerosvc.Event,err error) {
	id := MapBytesToTopicTitle(r.RngBlob(16))
	path = r.MQPrefix + "reply/" + util.GetFQDN() + "/" + id
	rspCh, err :=  r.Node.GetEventsCh(path + "/#")
	return path, rspCh,err
}

func (r *Runtime)RngBlob(bytes int) []byte {
	rnd := make([]byte,bytes)
	i,err := rand.Read(rnd)
	if err == nil && i == bytes {return rnd}
	var errctr uint8
	var readctr = i
	for {
		errctr++
		if errctr > 10 {
			log.Panicf("could not get data from RNG")
		}
		i, err := rand.Read(rnd[readctr:])
		if i > 0 { readctr += i } else {r.Log.Errorf("error getting RNG: %s",err)}
		if readctr >= bytes { return rnd }
	}
}

func (r *Runtime)SeededPRNG() *mathrand.Rand {
	blob :=r.RngBlob(8)
	seed := binary.BigEndian.Uint64(blob)
	return mathrand.New(mathrand.NewSource(int64(seed)))
}
func (r *Runtime)UnlikelyErr(err error, extra ...string) {
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
	return base64Replacer.Replace(strings.Trim(str,"="))
}