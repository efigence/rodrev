package downtime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a host may only downtime itself, and the topic is the only statement of who
// sent the request that can be trusted
func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name       string
		req        DowntimeRequest
		routingKey string
		errLike    string
		wantReason string
	}{
		{
			name:       "host downtiming itself",
			req:        DowntimeRequest{Host: "node1", Duration: time.Hour, Reason: "maintenance"},
			routingKey: "rv/downtime/node1",
			wantReason: "maintenance",
		},
		{
			name:       "certname is a fqdn",
			req:        DowntimeRequest{Host: "node1", Duration: time.Hour},
			routingKey: "rv/downtime/node1.example.com",
			wantReason: "not specified",
		},
		{
			name:       "certname carries the client prefix",
			req:        DowntimeRequest{Host: "node1", Duration: time.Hour},
			routingKey: "rv/downtime/client_node1.example.com",
			wantReason: "not specified",
		},
		{
			name:       "downtiming somebody else is refused",
			req:        DowntimeRequest{Host: "node2", Duration: time.Hour},
			routingKey: "rv/downtime/node1.example.com",
			errLike:    "only allowed to downtime itself",
		},
		{
			name:       "a prefix of another host is not that host",
			req:        DowntimeRequest{Host: "node1", Duration: time.Hour},
			routingKey: "rv/downtime/node11.example.com",
			errLike:    "only allowed to downtime itself",
		},
		{
			name:       "no host",
			req:        DowntimeRequest{Duration: time.Hour},
			routingKey: "rv/downtime/node1",
			errLike:    "no hostname",
		},
		{
			name:       "no duration",
			req:        DowntimeRequest{Host: "node1"},
			routingKey: "rv/downtime/node1",
			errLike:    "has to be positive",
		},
		{
			name:       "negative duration",
			req:        DowntimeRequest{Host: "node1", Duration: -time.Hour},
			routingKey: "rv/downtime/node1",
			errLike:    "has to be positive",
		},
		{
			name:       "longer than the limit",
			req:        DowntimeRequest{Host: "node1", Duration: maxDowntime},
			routingKey: "rv/downtime/node1",
			errLike:    "longer than the",
		},
		{
			name:       "just under the limit is fine",
			req:        DowntimeRequest{Host: "node1", Duration: maxDowntime - time.Second},
			routingKey: "rv/downtime/node1",
			wantReason: "not specified",
		},
		{
			name:       "whitespace reason counts as none",
			req:        DowntimeRequest{Host: "node1", Duration: time.Hour, Reason: "   "},
			routingKey: "rv/downtime/node1",
			wantReason: "not specified",
		},
		{
			name:       "reason is trimmed",
			req:        DowntimeRequest{Host: "node1", Duration: time.Hour, Reason: "  disk swap\n"},
			routingKey: "rv/downtime/node1",
			wantReason: "disk swap",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateRequest(tt.req, tt.routingKey)
			if len(tt.errLike) > 0 {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errLike)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReason, got.Reason)
			assert.Equal(t, tt.req.Host, got.Host)
		})
	}
}

func TestHostFromRoutingKey(t *testing.T) {
	tests := map[string]string{
		"rv/downtime/node1":                    "node1",
		"rv/downtime/node1.example.com":        "node1",
		"rv/downtime/client_node1.example.com": "node1",
		"rv/downtime/client_node1":             "node1",
		"node1":                                "node1",
		"":                                     "",
		"rv/downtime/":                         "",
	}
	for topic, want := range tests {
		assert.Equal(t, want, HostFromRoutingKey(topic), topic)
	}
}
