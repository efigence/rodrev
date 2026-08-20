package puppet

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSnapshot(t *testing.T) *Snapshot {
	t.Helper()
	s, err := LoadSnapshotFiles(
		filepath.Join(testDataDir, HarnessFactsFile),
		filepath.Join(testDataDir, HarnessClassfile),
		filepath.Join(testDataDir, HarnessLastRunSummaryFile),
	)
	require.NoError(t, err)
	return s
}

func TestLoadSnapshotFiles(t *testing.T) {
	s := testSnapshot(t)
	assert.Equal(t, "d1-lg.example.com", s.FQDN)
	assert.Equal(t, "kvm", s.Facts["virtual"])
	assert.Contains(t, s.Classes, "nginx")
	require.NotNil(t, s.LastRunSummary)
	assert.Equal(t, 1041, s.LastRunSummary.Resources.Total)
	assert.False(t, s.TS.IsZero())
	t.Run("facts only", func(t *testing.T) {
		s, err := LoadSnapshotFiles(filepath.Join(testDataDir, HarnessFactsFile), "", "")
		require.NoError(t, err)
		assert.Empty(t, s.Classes)
		assert.Nil(t, s.LastRunSummary)
	})
	t.Run("no fact file", func(t *testing.T) {
		_, err := LoadSnapshotFiles("", "", "")
		assert.Error(t, err)
		_, err = LoadSnapshotFiles(filepath.Join(t.TempDir(), "nope.yaml"), "", "")
		assert.Error(t, err)
	})
}

// a snapshot evaluates queries the same way the files it came from do
func TestSnapshotHarness(t *testing.T) {
	fromFiles, err := NewQueryHarness(testDataDir, nil)
	require.NoError(t, err)
	fromSnapshot, err := testSnapshot(t).Harness(nil)
	require.NoError(t, err)
	for _, tt := range queryTests {
		t.Run(tt.query, func(t *testing.T) {
			want, wantErr := fromFiles.Query(tt.query)
			got, gotErr := fromSnapshot.Query(tt.query)
			assert.Equal(t, want, got)
			assert.Equal(t, wantErr == nil, gotErr == nil)
		})
	}
	assert.Equal(t, fromFiles.FQDN(), fromSnapshot.FQDN())
	assert.Equal(t, fromFiles.LastRunSummary.Resources.Total, fromSnapshot.LastRunSummary.Resources.Total)
}

func TestNewQueryHarnessOpts(t *testing.T) {
	t.Run("individual files, no last run summary", func(t *testing.T) {
		h, err := NewQueryHarnessOpts(HarnessOptions{
			FactsPath:   filepath.Join(testDataDir, HarnessFactsFile),
			ClassesPath: filepath.Join(testDataDir, HarnessClassfile),
		})
		require.NoError(t, err)
		match, err := h.Query(`(and (== (class "nginx") true) (== (fact "virtual") "kvm"))`)
		require.NoError(t, err)
		assert.True(t, match)
	})
	t.Run("facts only, classes are optional", func(t *testing.T) {
		h, err := NewQueryHarnessOpts(HarnessOptions{
			FactsPath: filepath.Join(testDataDir, HarnessFactsFile),
		})
		require.NoError(t, err)
		match, err := h.Query(`(== (class "nginx") true)`)
		require.NoError(t, err)
		assert.False(t, match, "no classfile means no classes, not an error")
	})
	t.Run("in memory", func(t *testing.T) {
		h, err := NewQueryHarnessOpts(HarnessOptions{
			Facts:   map[string]interface{}{"virtual": "kvm"},
			Classes: []string{"nginx", ""},
			Source:  "test",
		})
		require.NoError(t, err)
		assert.Equal(t, "test", h.Source)
		for _, tt := range []struct {
			query string
			want  bool
		}{
			{`(== (fact "virtual") "kvm")`, true},
			{`(== (class "nginx") true)`, true},
			{`(== (class "apache") true)`, false},
		} {
			got, err := h.Query(tt.query)
			require.NoError(t, err, tt.query)
			assert.Equal(t, tt.want, got, tt.query)
		}
	})
	t.Run("needs facts", func(t *testing.T) {
		_, err := NewQueryHarnessOpts(HarnessOptions{Classes: []string{"nginx"}})
		assert.Error(t, err)
	})
	t.Run("node meta overrides generated one", func(t *testing.T) {
		h, err := NewQueryHarnessOpts(HarnessOptions{
			FactsPath: filepath.Join(testDataDir, HarnessFactsFile),
			NodeMeta:  map[string]interface{}{"fqdn": "other.example.com"},
		})
		require.NoError(t, err)
		match, err := h.Query(`(== (-> node %fqdn) "other.example.com")`)
		require.NoError(t, err)
		assert.True(t, match)
	})
}

func TestClassesList(t *testing.T) {
	c := NewClassesFromList([]string{"b", "a", " c ", "", "  "})
	assert.Equal(t, []string{"a", "b", "c"}, c.List())
	// no path to reload from
	assert.Error(t, c.UpdateClasses())
	f := NewFactsFromMap(map[string]interface{}{"a": 1})
	assert.Equal(t, 1, (*f.Map())["a"])
	assert.Error(t, f.UpdateFacts())
	assert.Empty(t, *NewFactsFromMap(nil).Map())
}
