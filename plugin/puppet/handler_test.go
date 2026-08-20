package puppet

import (
	"encoding/json"
	"testing"

	"github.com/efigence/rodrev/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// testPuppet builds a Puppet handler around the t-data node, without a puppet
// binary, a daemon or an MQ connection - New() needs all three, handleCommand
// needs none of them
func testPuppet(t *testing.T) *Puppet {
	t.Helper()
	h, err := NewQueryHarness(testDataDir, nil)
	require.NoError(t, err)
	var p Puppet
	p.facts = h.Facts
	p.classes = h.Classes
	p.lastRunSummary = h.LastRunSummary
	p.query = h.Engine
	p.fqdn = h.FQDN()
	p.l = zap.NewNop().Sugar()
	return &p
}

func broadcast() []string { return []string{"rv", "puppet"} }
func unicast(fqdn string) []string {
	return []string{"rv", "puppet", fqdn}
}

func cmdWith(t *testing.T, command, filter string, params interface{}) PuppetCmdRecv {
	t.Helper()
	cmd := PuppetCmdRecv{Command: command, Filter: filter}
	if params != nil {
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		cmd.Parameters = raw
	}
	return cmd
}

func TestHandleQuery(t *testing.T) {
	p := testPuppet(t)
	tests := []struct {
		name    string
		query   string
		matched bool
		errLike string
		typ     string
	}{
		{name: "match", query: `(== (class "nginx") true)`, matched: true, typ: "bool"},
		{name: "no match", query: `(== (class "apache") true)`, matched: false, typ: "bool"},
		{name: "nested fact", query: `(== (fact "os" "distro" "codename") "bookworm")`, matched: true, typ: "bool"},
		{name: "broken query", query: `(clas "nginx")`, errLike: "symbol `clas` not found"},
		{name: "not a boolean", query: `(fact "os")`, errLike: "not a boolean", typ: "hash"},
		{name: "empty", query: "", errLike: "empty query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := p.handleCommand(cmdWith(t, Query, tt.query, nil), broadcast())
			require.NoError(t, err)
			// every node answers a query, that is the whole point of it
			require.True(t, res.Send)
			assert.Equal(t, common.PuppetQuery, res.ReplyType)
			reply, ok := res.Body.(*QueryReply)
			require.True(t, ok)
			assert.Equal(t, p.fqdn, reply.FQDN)
			assert.Equal(t, tt.matched, reply.Matched)
			if len(tt.errLike) > 0 {
				assert.Contains(t, reply.Error, tt.errLike)
			} else {
				assert.Empty(t, reply.Error)
			}
			if len(tt.typ) > 0 {
				assert.Equal(t, tt.typ, reply.Type)
			}
		})
	}
}

// a filter that does not parse must be reported, not silently ignored: silence
// is indistinguishable from "did not match"
func TestFilterGate(t *testing.T) {
	p := testPuppet(t)
	t.Run("matching filter runs the command", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, Status, `(== (class "nginx") true)`, nil), broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetRunSummary, res.ReplyType)
	})
	t.Run("non matching filter stays quiet", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, Status, `(== (class "apache") true)`, nil), broadcast())
		require.NoError(t, err)
		assert.False(t, res.Send)
	})
	t.Run("broken filter answers with an error", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, Status, `(clas "nginx")`, nil), broadcast())
		assert.Error(t, err, "still logged locally")
		require.True(t, res.Send)
		assert.Equal(t, common.Error, res.ReplyType)
		msg, ok := res.Body.(*Msg)
		require.True(t, ok)
		assert.Contains(t, msg.Msg, "filter error")
	})
}

// answer_always turns silence into a countable answer, so a client learns how
// many nodes actually ran the filter without asking heartbeats who exists
func TestAnswerAlways(t *testing.T) {
	p := testPuppet(t)
	noMatch := `(== (class "apache") true)`
	t.Run("off: stays quiet", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, Status, noMatch, nil), broadcast())
		require.NoError(t, err)
		assert.False(t, res.Send)
	})
	t.Run("on: says it did not match", func(t *testing.T) {
		cmd := cmdWith(t, Status, noMatch, nil)
		cmd.AnswerAlways = true
		res, err := p.handleCommand(cmd, broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetNoMatch, res.ReplyType)
		reply, ok := res.Body.(*QueryReply)
		require.True(t, ok)
		assert.Equal(t, p.fqdn, reply.FQDN)
		assert.False(t, reply.Matched)
	})
	t.Run("on: a match is answered as usual", func(t *testing.T) {
		cmd := cmdWith(t, Status, `(== (class "nginx") true)`, nil)
		cmd.AnswerAlways = true
		res, err := p.handleCommand(cmd, broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetRunSummary, res.ReplyType)
	})
	t.Run("on: a broken filter is still an error, not a no-match", func(t *testing.T) {
		cmd := cmdWith(t, Status, `(clas "nginx")`, nil)
		cmd.AnswerAlways = true
		res, err := p.handleCommand(cmd, broadcast())
		assert.Error(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.Error, res.ReplyType)
	})
	t.Run("on: applies to any command", func(t *testing.T) {
		cmd := cmdWith(t, Fact, noMatch, FactOptions{Name: "virtual"})
		cmd.AnswerAlways = true
		res, err := p.handleCommand(cmd, broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetNoMatch, res.ReplyType)
	})
	// an unfiltered request has nothing to not match
	t.Run("on: no filter means the command just runs", func(t *testing.T) {
		cmd := cmdWith(t, Status, "", nil)
		cmd.AnswerAlways = true
		res, err := p.handleCommand(cmd, broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetRunSummary, res.ReplyType)
	})
}

func TestHandleFactList(t *testing.T) {
	p := testPuppet(t)
	t.Run("addressed to this node", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, FactList, "", nil), unicast(p.fqdn))
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetFacts, res.ReplyType)
		reply, ok := res.Body.(*FactsReply)
		require.True(t, ok)
		assert.Equal(t, p.fqdn, reply.FQDN)
		assert.Equal(t, "kvm", reply.Facts["virtual"])
		assert.Equal(t, len(reply.Facts), reply.Count)
		assert.False(t, reply.TS.IsZero())
	})
	t.Run("subset of keys", func(t *testing.T) {
		res, err := p.handleCommand(
			cmdWith(t, FactList, "", FactListOptions{Keys: []string{"virtual", "nope"}}),
			unicast(p.fqdn))
		require.NoError(t, err)
		reply := res.Body.(*FactsReply)
		assert.Equal(t, map[string]interface{}{"virtual": "kvm"}, reply.Facts)
		assert.Equal(t, 1, reply.Count)
	})
	// a fleet-wide fact dump is tens of kilobytes per node, so it needs either a
	// target or a filter
	t.Run("bare broadcast is refused", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, FactList, "", nil), broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.Error, res.ReplyType)
		assert.Contains(t, res.Body.(*Msg).Msg, "needs to be addressed at a node")
	})
	t.Run("broadcast with a filter is allowed", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, FactList, `(== (class "nginx") true)`, nil), broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.PuppetFacts, res.ReplyType)
	})
	// a unicast reaches every daemon, so a dump aimed at another node has to be
	// answered by nobody but that node
	t.Run("unicast to another node stays quiet", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, FactList, "", nil), unicast("other.example.com"))
		require.NoError(t, err)
		assert.False(t, res.Send)
	})
}

func TestHandleClassList(t *testing.T) {
	p := testPuppet(t)
	t.Run("bare broadcast is refused", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, ClassList, "", nil), broadcast())
		require.NoError(t, err)
		require.True(t, res.Send)
		assert.Equal(t, common.Error, res.ReplyType)
	})
	t.Run("unicast to another node stays quiet", func(t *testing.T) {
		res, err := p.handleCommand(cmdWith(t, ClassList, "", nil), unicast("other.example.com"))
		require.NoError(t, err)
		assert.False(t, res.Send)
	})
	res, err := p.handleCommand(cmdWith(t, ClassList, "", nil), unicast(p.fqdn))
	require.NoError(t, err)
	require.True(t, res.Send)
	assert.Equal(t, common.PuppetClasses, res.ReplyType)
	reply, ok := res.Body.(*ClassesReply)
	require.True(t, ok)
	assert.Contains(t, reply.Classes, "nginx")
	assert.Equal(t, p.fqdn, reply.FQDN)
	// sorted, so a diff between two nodes is readable
	assert.IsIncreasing(t, reply.Classes)
}

func TestHandleFact(t *testing.T) {
	p := testPuppet(t)
	res, err := p.handleCommand(cmdWith(t, Fact, "", FactOptions{Name: "virtual"}), broadcast())
	require.NoError(t, err)
	require.True(t, res.Send)
	assert.Equal(t, common.PuppetFact, res.ReplyType)
	assert.Equal(t, map[string]interface{}{"virtual": "kvm"}, res.Body)
}

func TestHandleUnknownCommand(t *testing.T) {
	p := testPuppet(t)
	res, err := p.handleCommand(cmdWith(t, "nosuchcommand", "", nil), broadcast())
	require.NoError(t, err)
	require.True(t, res.Send)
	assert.Equal(t, common.Error, res.ReplyType)
	assert.Contains(t, res.Body.(*Msg).Msg, "unknown command nosuchcommand")
}

func TestAddressedToMe(t *testing.T) {
	p := testPuppet(t)
	tests := []struct {
		name string
		path []string
		want bool
	}{
		{name: "broadcast", path: []string{"rv", "puppet"}, want: true},
		// matches the original len==2 broadcast rule; not a topic anyone uses
		{name: "two segment puppet path", path: []string{"puppet", "puppet"}, want: true},
		{name: "unicast to me", path: []string{"rv", "puppet", p.fqdn}, want: true},
		{name: "unicast to another node", path: []string{"rv", "puppet", "other.example.com"}, want: false},
		{name: "too short", path: []string{"puppet"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, p.addressedToMe(tt.path))
		})
	}
	assert.True(t, p.unicastToMe(unicast(p.fqdn)))
	assert.False(t, p.unicastToMe(unicast("other.example.com")))
	assert.False(t, p.unicastToMe(broadcast()))
	assert.True(t, unicastToOther(unicast("other.example.com")))
	assert.False(t, unicastToOther(broadcast()))
}

func TestFactSubset(t *testing.T) {
	facts := map[string]interface{}{"a": 1, "b": 2}
	assert.Equal(t, facts, factSubset(facts, nil))
	assert.Equal(t, map[string]interface{}{"a": 1}, factSubset(facts, []string{"a", "missing"}))
	assert.Empty(t, factSubset(facts, []string{"missing"}))
}

func TestSnapshotFromReplies(t *testing.T) {
	facts := &FactsReply{FQDN: "a.example.com", Facts: map[string]interface{}{"virtual": "kvm"}}
	classes := &ClassesReply{FQDN: "a.example.com", Classes: []string{"nginx"}}
	s := SnapshotFromReplies(facts, classes)
	assert.Equal(t, "a.example.com", s.FQDN)
	assert.Equal(t, "node:a.example.com", s.Source)
	assert.Equal(t, "kvm", s.Facts["virtual"])
	assert.Equal(t, []string{"nginx"}, s.Classes)
	// a node that only answered one of the two still gives a usable snapshot
	assert.Equal(t, "a.example.com", SnapshotFromReplies(nil, classes).FQDN)
	assert.Equal(t, "a.example.com", SnapshotFromReplies(facts, nil).FQDN)
}
