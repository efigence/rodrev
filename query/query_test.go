package query

import (
	"github.com/efigence/rodrev/common"
	"github.com/efigence/rodrev/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParseBoolNodeMeta(t *testing.T) {
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

func testEngine(t *testing.T) *Engine {
	t.Helper()
	r := &common.Runtime{
		Cfg: config.Config{
			NodeMeta: map[string]interface{}{
				"fqdn": "example.com",
				"site": "test",
			},
		},
	}
	q := NewQueryEngine(r)
	facts := map[string]interface{}{
		"virtual": "kvm",
		"count":   4,
		"os":      map[string]interface{}{"name": "Debian"},
	}
	require.NoError(t, q.RegisterMap("fact", mapGetter{&facts}))
	return q
}

type mapGetter struct {
	m *map[string]interface{}
}

func (m mapGetter) Map() *map[string]interface{} { return m.m }

func TestParse(t *testing.T) {
	q := testEngine(t)
	tests := []struct {
		query   string
		typ     string
		boolish bool
		val     bool
		display string
		wantErr bool
	}{
		{query: `(== (fact "virtual") "kvm")`, typ: "bool", boolish: true, val: true, display: "true"},
		{query: `(== (fact "virtual") "vmware")`, typ: "bool", boolish: true, val: false, display: "false"},
		{query: `(fact "virtual")`, typ: "string", boolish: true, val: true, display: `"kvm"`},
		{query: `(fact "count")`, typ: "int", boolish: true, val: true, display: "4"},
		{query: `(fact "nonexistent")`, typ: "nil", boolish: false},
		// a hash is not a filter, but the REPL still wants to show it
		{query: `(fact "os")`, typ: "hash", boolish: false},
		{query: `(fact`, wantErr: true},
		{query: `(nosuchfunction "x")`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			res, err := q.Parse(tt.query)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.typ, res.Type)
			assert.Equal(t, tt.boolish, res.Boolish)
			assert.Equal(t, tt.val, res.Bool)
			if len(tt.display) > 0 {
				assert.Equal(t, tt.display, res.Display)
			}
			// ParseBool agrees with Parse, and still rejects non-booleans
			b, err := q.ParseBool(tt.query)
			if tt.boolish {
				require.NoError(t, err)
				assert.Equal(t, tt.val, b)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestCheckSyntax(t *testing.T) {
	valid := []string{
		`(== (class "nginx") true)`,
		`(and (== (fact "virtual") "kvm") (regexp (-> node %fqdn) "^d1-"))`,
		// unknown functions and facts only fail when actually run, on the node
		`(nosuchfunction "x")`,
		`(fact "whatever")`,
	}
	for _, q := range valid {
		assert.NoError(t, CheckSyntax(q), q)
	}
	invalid := []string{
		`(== (class "nginx" true)`,
		`(== (class "nginx") true))`,
		`(== (class "unterminated`,
	}
	for _, q := range invalid {
		assert.Error(t, CheckSyntax(q), q)
	}
}

func TestCompletions(t *testing.T) {
	c := Completions()
	assert.Contains(t, c, "regexp")
	assert.Contains(t, c, "node")
	assert.Contains(t, c, "and")
	assert.Contains(t, c, "not")
	// returned slice is a copy, callers can not corrupt the cache
	c[0] = "clobbered"
	assert.NotContains(t, Completions(), "clobbered")
}

func TestDataMaps(t *testing.T) {
	q := testEngine(t)
	assert.Equal(t, []string{"fact"}, q.DataMaps())
}
