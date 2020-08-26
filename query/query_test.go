package query

import (
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParse(t *testing.T) {
	r := &common.Runtime{
		Node:     nil,
		FQDN:     "",
		MQPrefix: "",
		Cfg: config.Config{
			NodeMeta: map[string]interface{}{
				"fqdn": "example.com",
				"site": "test",
			},
		},
		Log: nil,
	}
	q := NewQueryEngine(r)
	a, err := q.ParseBool(`(==(-> node %fqdn) "example.com")`)
	require.NoError(t, err)
	assert.True(t, a)
	t.Logf("in: %+v", r.Cfg.NodeMeta)
	b, err := q.ParseBool(`(==(-> node %fqdn) "example.moc")`)
	require.NoError(t, err)
	assert.False(t, b)

}
