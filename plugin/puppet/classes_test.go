package puppet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadClasses(t *testing.T) {
	c, err := LoadClasses(filepath.Join(testDataDir, HarnessClassfile))
	require.NoError(t, err)
	classes := *c.Map()
	assert.Equal(t, true, classes["systemd::common"])
	assert.Equal(t, true, classes["nginx"])
	assert.Nil(t, classes["nonexistent::class"])
	assert.Nil(t, classes[""])
}

func TestLoadClassesErrors(t *testing.T) {
	dir := t.TempDir()
	t.Run("nonexistent file", func(t *testing.T) {
		c, err := LoadClasses(filepath.Join(dir, "nonexistent.txt"))
		require.Error(t, err)
		// object is still usable, just empty
		require.NotNil(t, c)
		assert.Empty(t, *c.Map())
	})
	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		require.NoError(t, os.WriteFile(path, []byte(""), 0644))
		_, err := LoadClasses(path)
		assert.Error(t, err)
	})
	t.Run("whitespace only file", func(t *testing.T) {
		path := filepath.Join(dir, "whitespace.txt")
		require.NoError(t, os.WriteFile(path, []byte("\n \n\t\n"), 0644))
		_, err := LoadClasses(path)
		assert.Error(t, err)
	})
	t.Run("keep old classes on failed update", func(t *testing.T) {
		path := filepath.Join(dir, "classes.txt")
		require.NoError(t, os.WriteFile(path, []byte("a::b\nc::d\n"), 0644))
		c, err := LoadClasses(path)
		require.NoError(t, err)
		require.Len(t, *c.Map(), 2)
		// puppet truncating the file mid-write should not wipe out the class list
		require.NoError(t, os.WriteFile(path, []byte(""), 0644))
		assert.Error(t, c.UpdateClasses())
		assert.Len(t, *c.Map(), 2)
		require.NoError(t, os.Remove(path))
		assert.Error(t, c.UpdateClasses())
		assert.Len(t, *c.Map(), 2)
	})
}
