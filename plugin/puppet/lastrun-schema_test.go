package puppet

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestParseLastRunSummary(t *testing.T) {
	fd, err := os.Open("../../t-data/last_run_summary.yaml")
	require.NoError(t, err)
	defer fd.Close()
	summary, err := ParseLastRunSummary(fd)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, 3, summary.Events.Total)
	assert.Equal(t, 1041, summary.Resources.Total)
	assert.InDelta(t, 28, summary.Timing.Duration["config_retrieval"], 1)
	assert.Equal(t, 1585892661, summary.Timing.LastRun)
}
