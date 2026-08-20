package fence

import (
	"testing"

	"github.com/efigence/rodrev/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zerosvc/go-zerosvc"
	"go.uber.org/zap"
)

const testFQDN = "node1.example.com"

func testFence(cfg config.FenceConfig) *Fence {
	var f Fence
	f.cfg = &cfg
	f.l = zap.NewNop().Sugar()
	f.fqdn = testFQDN
	return &f
}

// fenceRequest is what a client sends: it names the target and may claim a group
func fenceRequest(from string, group string, target string) (*zerosvc.Event, *FenceCmd) {
	ev := zerosvc.Event{NodeName: from, Headers: map[string]any{}}
	if len(group) > 0 {
		ev.Headers["fence-group"] = group
	}
	return &ev, &FenceCmd{Command: cmdFence, Node: target}
}

// fencing is deliberately weakly guarded (there is no password check yet), so
// these pin down exactly how much a request has to prove
func TestCheckPermissions(t *testing.T) {
	t.Run("no acl configured lets everything through", func(t *testing.T) {
		f := testFence(config.FenceConfig{Enabled: true})
		ev, cmd := fenceRequest("rf-client-somewhere", "", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
	t.Run("matching group is allowed", func(t *testing.T) {
		f := testFence(config.FenceConfig{Enabled: true, Group: "sql"})
		ev, cmd := fenceRequest("rf-client-somewhere", "sql", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed, "claiming the group is all it takes, the password is not checked yet")
	})
	t.Run("wrong group is refused", func(t *testing.T) {
		f := testFence(config.FenceConfig{Enabled: true, Group: "sql"})
		ev, cmd := fenceRequest("rf-client-somewhere", "web", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.False(t, allowed)
	})
	t.Run("no group header is refused", func(t *testing.T) {
		f := testFence(config.FenceConfig{Enabled: true, Group: "sql"})
		ev, cmd := fenceRequest("rf-client-somewhere", "", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.False(t, allowed)
	})
}

// node_map is keyed by the client that asks, and its entry lists the nodes that
// client may fence - exactly the config the README documents
func TestCheckPermissionsNodeMap(t *testing.T) {
	documented := config.FenceConfig{
		Enabled: true,
		NodeMap: map[string]config.FenceNode{
			"rf-client-node1-fence": {Nodes: []string{testFQDN, "node2.example.com"}},
		},
	}
	t.Run("a client listed for us is allowed", func(t *testing.T) {
		f := testFence(documented)
		ev, cmd := fenceRequest("rf-client-node1-fence", "", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
	t.Run("a client with no entry is refused", func(t *testing.T) {
		f := testFence(documented)
		ev, cmd := fenceRequest("rf-client-stranger", "", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.False(t, allowed)
	})
	t.Run("a client whose entry does not list us is refused", func(t *testing.T) {
		f := testFence(config.FenceConfig{
			Enabled: true,
			NodeMap: map[string]config.FenceNode{
				"rf-client-node1-fence": {Nodes: []string{"node2.example.com"}},
			},
		})
		ev, cmd := fenceRequest("rf-client-node1-fence", "", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.False(t, allowed)
	})
	// the singular key is what earlier versions read, so a config written that
	// way keeps working
	t.Run("the deprecated node key still works", func(t *testing.T) {
		f := testFence(config.FenceConfig{
			Enabled: true,
			NodeMap: map[string]config.FenceNode{
				"rf-client-node1-fence": {Node: []string{testFQDN}},
			},
		})
		ev, cmd := fenceRequest("rf-client-node1-fence", "", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
	t.Run("group and node map together: either one is enough", func(t *testing.T) {
		f := testFence(config.FenceConfig{
			Enabled: true,
			Group:   "sql",
			NodeMap: map[string]config.FenceNode{
				"rf-client-node1-fence": {Nodes: []string{"node2.example.com"}},
			},
		})
		ev, cmd := fenceRequest("rf-client-node1-fence", "sql", testFQDN)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed, "the group passes even though the node map does not")
	})
}

// a request reaches us on our own fence topic, so one naming another node is
// not ours to act on even if the sender is otherwise allowed
func TestCheckPermissionsWrongTarget(t *testing.T) {
	cfg := config.FenceConfig{
		Enabled: true,
		Group:   "sql",
		NodeMap: map[string]config.FenceNode{
			"rf-client-node1-fence": {Nodes: []string{testFQDN}},
		},
	}
	for _, target := range []string{"node2.example.com", "somebody.else"} {
		f := testFence(cfg)
		ev, cmd := fenceRequest("rf-client-node1-fence", "sql", target)
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.False(t, allowed, target)
	}
	t.Run("a request that names nobody is still ours", func(t *testing.T) {
		f := testFence(cfg)
		ev, cmd := fenceRequest("rf-client-node1-fence", "sql", "")
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed, "routing already decided it was for us")
	})
	// with no acl at all the target check does not apply: nothing is configured
	// to be checked
	t.Run("no acl configured", func(t *testing.T) {
		f := testFence(config.FenceConfig{Enabled: true})
		ev, cmd := fenceRequest("rf-client-node1-fence", "", "node2.example.com")
		allowed, err := f.CheckPermissions(ev, cmd)
		require.NoError(t, err)
		assert.True(t, allowed)
	})
}
