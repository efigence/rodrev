package puppet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFacts(t *testing.T) {
	f, err := LoadFacts(filepath.Join(testDataDir, HarnessFactsFile))
	require.NoError(t, err)
	facts := *f.Map()
	assert.Equal(t, "kvm", facts["virtual"])
	assert.Nil(t, facts["nonexistent_fact"])
}

func TestLoadFactsErrors(t *testing.T) {
	dir := t.TempDir()
	t.Run("nonexistent file", func(t *testing.T) {
		f, err := LoadFacts(filepath.Join(dir, "nonexistent.yaml"))
		require.Error(t, err)
		// object is still usable, just empty
		require.NotNil(t, f)
		assert.Empty(t, *f.Map())
	})
	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.yaml")
		require.NoError(t, os.WriteFile(path, []byte("---\n"), 0644))
		_, err := LoadFacts(path)
		assert.Error(t, err)
	})
	t.Run("broken yaml", func(t *testing.T) {
		path := filepath.Join(dir, "broken.yaml")
		require.NoError(t, os.WriteFile(path, []byte("---\nkey: [unterminated\n"), 0644))
		_, err := LoadFacts(path)
		assert.Error(t, err)
	})
	t.Run("keep old facts on failed update", func(t *testing.T) {
		path := filepath.Join(dir, "facts.yaml")
		require.NoError(t, os.WriteFile(path, []byte("---\nvirtual: kvm\n"), 0644))
		f, err := LoadFacts(path)
		require.NoError(t, err)
		require.Len(t, *f.Map(), 1)
		// puppet truncating the file mid-write should not wipe out the facts
		require.NoError(t, os.WriteFile(path, []byte(""), 0644))
		assert.Error(t, f.UpdateFacts())
		assert.Equal(t, "kvm", (*f.Map())["virtual"])
		require.NoError(t, os.Remove(path))
		assert.Error(t, f.UpdateFacts())
		assert.Equal(t, "kvm", (*f.Map())["virtual"])
	})
}
