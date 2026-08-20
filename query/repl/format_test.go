package repl

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollapseErrors(t *testing.T) {
	sameErr := "error running query [x]: boom"
	tests := []struct {
		name    string
		results []NodeResult
		want    []string
	}{
		{name: "none", results: []NodeResult{{FQDN: "a", Matched: true}}, want: []string{}},
		{
			name:    "one",
			results: []NodeResult{{FQDN: "a", Err: sameErr}},
			want:    []string{"a: " + sameErr},
		},
		{
			name: "a few are listed",
			results: []NodeResult{
				{FQDN: "a", Err: sameErr}, {FQDN: "b", Err: sameErr},
			},
			want: []string{"a b: " + sameErr},
		},
		{
			name: "many are counted",
			results: []NodeResult{
				{FQDN: "a", Err: sameErr}, {FQDN: "b", Err: sameErr},
				{FQDN: "c", Err: sameErr}, {FQDN: "d", Err: sameErr},
			},
			want: []string{"4 nodes (a ...): " + sameErr},
		},
		{
			name: "different errors stay apart",
			results: []NodeResult{
				{FQDN: "a", Err: "boom"}, {FQDN: "b", Err: "bang"},
			},
			want: []string{"a: boom", "b: bang"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, collapseErrors(tt.results))
		})
	}
}

func TestFormatCSV(t *testing.T) {
	var out bytes.Buffer
	s, err := New(Config{Backend: &fakeBackend{}, Out: &out, Format: FormatCSV})
	require.NoError(t, err)
	sum := Summary{Expr: `(== (class "nginx") true)`, Matched: 1, Responded: 2, Known: 2}
	results := []NodeResult{
		{FQDN: "a.example.com", Matched: true},
		{FQDN: "b.example.com", Err: "boom"},
	}
	require.NoError(t, s.printQuery(sum, results))
	// the header is written once per session, not once per query
	require.NoError(t, s.printQuery(sum, results[:1]))
	assert.Equal(t, `expr,fqdn,matched,error
"(== (class ""nginx"") true)",a.example.com,1,
"(== (class ""nginx"") true)",b.example.com,0,boom
"(== (class ""nginx"") true)",a.example.com,1,
`, out.String())
}

func TestFormatJSON(t *testing.T) {
	var out bytes.Buffer
	s, err := New(Config{Backend: &fakeBackend{}, Out: &out, Format: FormatJSON})
	require.NoError(t, err)
	sum := Summary{
		Expr: `(== (class "nginx") true)`, Matched: 1, Responded: 3, Known: 12,
		Errors: 1, Elapsed: time.Millisecond * 412, Type: "bool", Boolish: true,
	}
	results := []NodeResult{
		{FQDN: "b.example.com", Matched: true},
		{FQDN: "a.example.com"},
		{FQDN: "c.example.com", Err: "boom"},
	}
	require.NoError(t, s.printQuery(sum, results))
	// NDJSON: one self-contained record per query
	require.NoError(t, s.printQuery(sum, results))
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)
	var got queryJSON
	require.NoError(t, json.Unmarshal(lines[0], &got))
	assert.Equal(t, sum.Expr, got.Expr)
	assert.Equal(t, int64(412), got.ElapsedMs)
	assert.Equal(t, []string{"b.example.com"}, got.Matched)
	assert.Equal(t, []string{"a.example.com"}, got.Unmatched)
	assert.Equal(t, map[string]string{"c.example.com": "boom"}, got.Errors)
	assert.Equal(t, 12, got.Summary.Known)
	assert.Equal(t, 1, got.Summary.Errors)
}

func TestFactOutputFormats(t *testing.T) {
	t.Run("human", func(t *testing.T) {
		s, out := testSession(t, FormatHuman)
		require.NoError(t, s.EvalLine(":fact os.distro.codename"))
		assert.Equal(t, "os.distro.codename: bookworm\n", out.String())
	})
	t.Run("human nested", func(t *testing.T) {
		s, out := testSession(t, FormatHuman)
		require.NoError(t, s.EvalLine(":fact os.distro.release"))
		assert.Contains(t, out.String(), "os.distro.release:\n  full: \"12.8\"")
	})
	t.Run("csv", func(t *testing.T) {
		s, out := testSession(t, FormatCSV)
		require.NoError(t, s.EvalLine(":fact os.distro.codename"))
		assert.Equal(t, "fqdn,fact,value\nd1-lg.example.com,os.distro.codename,bookworm\n", out.String())
	})
	t.Run("json", func(t *testing.T) {
		s, out := testSession(t, FormatJSON)
		require.NoError(t, s.EvalLine(":fact os.distro.codename"))
		var got map[string]map[string]interface{}
		require.NoError(t, json.Unmarshal(out.Bytes(), &got))
		assert.Equal(t, "bookworm", got["d1-lg.example.com"]["os.distro.codename"])
	})
	t.Run("class csv", func(t *testing.T) {
		s, out := testSession(t, FormatCSV)
		require.NoError(t, s.EvalLine(":class mon::check::puppet"))
		assert.Equal(t, "fqdn,class\nd1-lg.example.com,mon::check::puppet\n", out.String())
	})
}

func TestRenderValue(t *testing.T) {
	assert.Equal(t, "kvm", renderValue("kvm"))
	assert.Equal(t, "4", renderValue(4))
	assert.Equal(t, "true", renderValue(true))
	assert.Equal(t, "(not set)", renderValue(nil))
	assert.Equal(t, "- a\n- b", renderValue([]interface{}{"a", "b"}))
	assert.Equal(t, "a: 1", renderValue(map[string]interface{}{"a": 1}))
}
