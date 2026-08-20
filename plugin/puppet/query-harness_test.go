package puppet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// example data set: facts.yaml, classes.txt and last_run_summary.yaml of a node
const testDataDir = "../../t-data"

// queryTest is a single CLI query (`rv puppet --filter ...`) run against testDataDir
type queryTest struct {
	query string
	// expected result of the query
	want bool
	// query is expected to fail (bad syntax, non-boolean result, ...)
	wantErr bool
}

// add a line here to test another query against the data in t-data/
var queryTests = []queryTest{
	// classes
	{query: `(== (class "systemd::common") true)`, want: true},
	{query: `(== (class "nginx") true)`, want: true},
	{query: `(== (class "core::security::cve_2026_46333") true)`, want: true},
	{query: `(== (class "nonexistent::class") true)`, want: false},
	{query: `(!= (class "nonexistent::class") true)`, want: true},
	// facts
	{query: `(== (fact "virtual") "kvm")`, want: true},
	{query: `(== (fact "virtual") "vmware")`, want: false},
	{query: `(fact "is_virtual")`, want: true},
	{query: `(== (fact "kernelmajversion") "6.1")`, want: true},
	{query: `(== (fact "nonexistent_fact") "whatever")`, want: false},
	// nested facts
	{query: `(== (fact "processors" "count") 4)`, want: true},
	{query: `(== (fact "os" "distro" "codename") "bookworm")`, want: true},
	{query: `(== (fact "networking" "fqdn") "d1-lg.example.com")`, want: true},
	{query: `(== (fact "networking" "interfaces" "eth0" "bindings" 0 "address") "10.100.101.33")`, want: true},
	// numeric comparison
	{query: `(> (fact "apt_dist_updates") 100)`, want: true},
	{query: `(< (fact "apt_dist_updates") 100)`, want: false},
	// node metadata
	{query: `(== (-> node %fqdn) "d1-lg.example.com")`, want: true},
	{query: `(regexp (-> node %fqdn) "^d1-")`, want: true},
	{query: `(regexp (-> node %fqdn) "^d2-")`, want: false},
	{query: `(== (-> node %accounting_project) "dc1-example")`, want: true},
	// combining
	{query: `(and (== (fact "virtual") "kvm") (== (class "nginx") true))`, want: true},
	{query: `(and (== (fact "virtual") "kvm") (== (class "apache") true))`, want: false},
	{query: `(or (== (class "apache") true) (== (class "nginx") true))`, want: true},
	// errors
	{query: `(fact "os")`, wantErr: true},               // hash is not a boolean
	{query: `(fact "nonexistent_fact")`, wantErr: true}, // nil is not a boolean
	{query: `(fact)`, wantErr: true},                    // missing arguments
	{query: `(class "nginx" `, wantErr: true},           // unterminated expression
}

func TestQueryHarness(t *testing.T) {
	h, err := NewQueryHarness(testDataDir, nil)
	require.NoError(t, err)
	require.NotEmpty(t, *h.Facts.Map())
	require.NotEmpty(t, *h.Classes.Map())
	assert.Equal(t, "d1-lg.example.com", h.FQDN())
	assert.Equal(t, 1041, h.LastRunSummary.Resources.Total)
	for _, tt := range queryTests {
		t.Run(tt.query, func(t *testing.T) {
			got, err := h.Query(tt.query)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// node metadata can be overriden to test queries against other node config
func TestQueryHarnessNodeMeta(t *testing.T) {
	h, err := NewQueryHarness(testDataDir, map[string]interface{}{
		"fqdn": "other.example.com",
		"site": "dc2",
	})
	require.NoError(t, err)
	got, err := h.Query(`(and (== (-> node %site) "dc2") (== (fact "virtual") "kvm"))`)
	require.NoError(t, err)
	assert.True(t, got)
}

func TestQueryHarnessMissingData(t *testing.T) {
	_, err := NewQueryHarness(filepath.Join(t.TempDir(), "nonexistent"), nil)
	assert.Error(t, err)
	// facts present, classfile missing
	dir := t.TempDir()
	factsIn, err := os.ReadFile(filepath.Join(testDataDir, HarnessFactsFile))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, HarnessFactsFile), factsIn, 0644))
	_, err = NewQueryHarness(dir, nil)
	assert.Error(t, err)
}
