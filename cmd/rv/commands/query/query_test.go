package query

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDataDir = "../../../../t-data"

// the file flags decide what a local session evaluates against
func TestLoadLocal(t *testing.T) {
	t.Run("data dir", func(t *testing.T) {
		snap, err := loadLocal("", testDataDir, "", "", "")
		require.NoError(t, err)
		assert.Equal(t, "d1-lg.example.com", snap.FQDN)
		assert.Equal(t, "kvm", snap.Facts["virtual"])
		assert.Contains(t, snap.Classes, "nginx")
		require.NotNil(t, snap.LastRunSummary, "a data dir has a run summary in it")
	})
	t.Run("individual files", func(t *testing.T) {
		snap, err := loadLocal("",
			"", filepath.Join(testDataDir, "facts.yaml"), filepath.Join(testDataDir, "classes.txt"), "")
		require.NoError(t, err)
		assert.Equal(t, "kvm", snap.Facts["virtual"])
		assert.Contains(t, snap.Classes, "nginx")
		assert.Nil(t, snap.LastRunSummary, "no summary was asked for")
	})
	t.Run("facts only", func(t *testing.T) {
		snap, err := loadLocal("", "", filepath.Join(testDataDir, "facts.yaml"), "", "")
		require.NoError(t, err)
		assert.Empty(t, snap.Classes)
	})
	t.Run("saved snapshot wins over the rest", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "node.json")
		require.NoError(t, os.WriteFile(path,
			[]byte(`{"fqdn":"saved.example.com","facts":{"virtual":"vmware"}}`), 0600))
		snap, err := loadLocal(path, testDataDir, "", "", "")
		require.NoError(t, err)
		assert.Equal(t, "saved.example.com", snap.FQDN)
		assert.Equal(t, "vmware", snap.Facts["virtual"])
	})
	t.Run("nothing to load", func(t *testing.T) {
		_, err := loadLocal("", "", "", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--facts")
	})
	t.Run("classes without facts", func(t *testing.T) {
		_, err := loadLocal("", "", "", filepath.Join(testDataDir, "classes.txt"), "")
		assert.Error(t, err, "a query needs facts, classes alone are not a data set")
	})
	t.Run("missing file", func(t *testing.T) {
		_, err := loadLocal("", "", filepath.Join(t.TempDir(), "nope.yaml"), "", "")
		assert.Error(t, err)
	})
	t.Run("last run summary is optional but has to exist when asked for", func(t *testing.T) {
		snap, err := loadLocal("", "",
			filepath.Join(testDataDir, "facts.yaml"), "",
			filepath.Join(testDataDir, "last_run_summary.yaml"))
		require.NoError(t, err)
		require.NotNil(t, snap.LastRunSummary)
		assert.Equal(t, 1041, snap.LastRunSummary.Resources.Total)

		_, err = loadLocal("", "", filepath.Join(testDataDir, "facts.yaml"), "", "/nonexistent/summary.yaml")
		assert.Error(t, err)
	})
}

func TestHistoryPath(t *testing.T) {
	assert.Equal(t, "", historyPath("/tmp/hist", true), "--no-history wins")
	assert.Equal(t, "", historyPath("", true))
	assert.Equal(t, "/tmp/hist", historyPath("/tmp/hist", false))
	// the default lands in the home dir
	def := historyPath("", false)
	if home, err := os.UserHomeDir(); err == nil && len(home) > 0 {
		assert.Equal(t, filepath.Join(home, ".rv_query_history"), def)
	}
}
