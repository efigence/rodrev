package util

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zerosvc/go-zerosvc"
)

// PPEvent is what --debug prints, so it has to show where an event came from and
// where its answer goes
func TestPPEvent(t *testing.T) {
	ev := zerosvc.Event{
		RoutingKey: "rv/puppet/d1-lg.example.com",
		ReplyTo:    "reply/rf-client-laptop/abc123",
		Headers:    map[string]any{"correlation-id": "call1", "fqdn": "d1-lg.example.com"},
		Body:       []byte(`{"cmd":"status"}`),
	}
	out := PPEvent(&ev)
	assert.Contains(t, out, "rv/puppet/d1-lg.example.com")
	assert.Contains(t, out, "reply/rf-client-laptop/abc123")
	assert.Contains(t, out, "correlation-id")
	assert.Contains(t, out, `{"cmd":"status"}`)
	assert.True(t, strings.HasSuffix(out, "\n"))

	// an empty event still prints rather than blowing up
	assert.NotPanics(t, func() { PPEvent(&zerosvc.Event{}) })
}
