package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRedactURL(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("", RedactURL(""))
	assert.Equal(
		"tcp://mqtt:xxxxx@127.0.0.1:1883",
		RedactURL("tcp://mqtt:mqtt@127.0.0.1:1883"),
	)
	assert.Equal(
		"tls://user:xxxxx@mq.example.com:8883/?ca=%2Fetc%2Fssl%2Fca.crt",
		RedactURL("tls://user:s3cr3t@mq.example.com:8883/?ca=%2Fetc%2Fssl%2Fca.crt"),
	)
	// no credentials to redact
	assert.Equal("tls://mq.example.com:8883", RedactURL("tls://mq.example.com:8883"))
	assert.Equal("tls://user@mq.example.com:8883", RedactURL("tls://user@mq.example.com:8883"))
	assert.Equal("[unparseable url]", RedactURL("tcp://mqtt:p%ss@127.0.0.1:1883"))
}
