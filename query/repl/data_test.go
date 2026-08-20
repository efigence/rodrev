package repl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var testFacts = map[string]interface{}{
	"virtual": "kvm",
	"os": map[string]interface{}{
		"distro": map[string]interface{}{"codename": "bookworm"},
		"family": "Debian",
	},
	"networking": map[string]interface{}{
		"interfaces": []interface{}{
			map[string]interface{}{"address": "10.0.0.1"},
			map[string]interface{}{"address": "10.0.0.2"},
		},
	},
}

func TestFactPath(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want interface{}
		ok   bool
	}{
		{name: "flat", path: []string{"virtual"}, want: "kvm", ok: true},
		{name: "nested", path: []string{"os", "distro", "codename"}, want: "bookworm", ok: true},
		{name: "array index", path: []string{"networking", "interfaces", "1", "address"}, want: "10.0.0.2", ok: true},
		{name: "missing key", path: []string{"nope"}, ok: false},
		{name: "missing nested", path: []string{"os", "nope"}, ok: false},
		{name: "index out of range", path: []string{"networking", "interfaces", "9"}, ok: false},
		{name: "index into a map", path: []string{"os", "0"}, ok: false},
		{name: "too deep", path: []string{"virtual", "more"}, ok: false},
		{name: "empty path returns everything", path: nil, want: testFacts, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := factPath(testFacts, tt.path)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFactChildren(t *testing.T) {
	assert.Equal(t, []string{"networking", "os", "virtual"}, factChildren(testFacts, nil))
	assert.Equal(t, []string{"distro", "family"}, factChildren(testFacts, []string{"os"}))
	assert.Equal(t, []string{"0", "1"}, factChildren(testFacts, []string{"networking", "interfaces"}))
	assert.Empty(t, factChildren(testFacts, []string{"virtual"}))
	assert.Empty(t, factChildren(testFacts, []string{"nope"}))
}

func TestMatchNames(t *testing.T) {
	names := []string{"apt_updates", "apt_has_updates", "virtual", "mon::check::puppet"}
	assert.Equal(t, names, matchNames(names, ""))
	assert.Equal(t, []string{"apt_updates", "apt_has_updates"}, matchNames(names, "apt"))
	assert.Equal(t, []string{"apt_has_updates"}, matchNames(names, "apt_has*"))
	assert.Equal(t, []string{"mon::check::puppet"}, matchNames(names, "mon::*"))
	assert.Empty(t, matchNames(names, "nope"))
	// a broken glob does not match rather than blowing up
	assert.Empty(t, matchNames(names, "[a-"))
}
