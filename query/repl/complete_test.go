package repl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// completionState builds a state from the t-data node
func testCompletionState(t *testing.T) CompletionState {
	t.Helper()
	s, _ := testSession(t, FormatHuman)
	return s.CompletionState()
}

func TestComplete(t *testing.T) {
	st := testCompletionState(t)
	tests := []struct {
		name string
		line string
		// pos defaults to the end of line
		pos      int
		head     string
		tail     string
		contains []string
		absent   []string
		empty    bool
	}{
		{
			name: "meta command", line: ":cl",
			head: ":", contains: []string{"class ", "cluster "}, absent: []string{"fact "},
		},
		{
			name: "meta command all", line: ":",
			head: ":", contains: []string{"help ", "syntax ", "quit "},
		},
		{
			name: "meta arg out", line: ":out j",
			head: ":out ", contains: []string{"json "}, absent: []string{"csv "},
		},
		{
			name: "meta arg fact", line: ":fact virt",
			head: ":fact ", contains: []string{"virtual "},
		},
		{
			name: "meta arg class", line: ":class ngin",
			head: ":class ", contains: []string{"nginx "},
		},
		{
			name: "meta arg none typed yet", line: ":fact ",
			head: ":fact ", contains: []string{"virtual ", "os "},
		},
		{
			name: "meta with no completions", line: ":verbose ",
			head: ":verbose ", empty: true,
		},
		{
			name: "function after paren", line: "(fa",
			head: "(", contains: []string{"fact "},
		},
		{
			name: "function all", line: "(",
			head: "(", contains: []string{"fact ", "class ", "regexp "},
		},
		{
			name: "fact name in string", line: `(== (fact "virt`,
			head: `(== (fact "`, contains: []string{`virtual"`},
		},
		{
			name: "fact name empty string", line: `(fact "`,
			head: `(fact "`, contains: []string{`virtual"`, `os"`},
		},
		{
			name: "nested fact path", line: `(fact "os" "distro" "co`,
			head: `(fact "os" "distro" "`, contains: []string{`codename"`}, absent: []string{`virtual"`},
		},
		{
			name: "array index as path segment", line: `(fact "networking" "interfaces" "eth0" "bindings" "`,
			head: `(fact "networking" "interfaces" "eth0" "bindings" "`, contains: []string{`0"`},
		},
		{
			name: "class segment", line: `(class "mon::che`,
			head: `(class "`, contains: []string{"mon::check::"}, absent: []string{`mon::check::puppet"`},
		},
		{
			name: "class full name", line: `(class "mon::check::pup`,
			head: `(class "`, contains: []string{`mon::check::puppet"`},
		},
		{
			name: "node meta key", line: "(-> node %f",
			head: "(-> node ", contains: []string{"%fqdn "},
		},
		{
			name: "cursor in the middle", line: `(== (fact "virt") "kvm")`, pos: 15,
			head: `(== (fact "`, tail: `") "kvm")`, contains: []string{`virtual"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := tt.pos
			if pos == 0 {
				pos = len([]rune(tt.line))
			}
			head, completions, tail := Complete(st, tt.line, pos)
			assert.Equal(t, tt.head, head, "head")
			wantTail := tt.tail
			assert.Equal(t, wantTail, tail, "tail")
			if tt.empty {
				assert.Empty(t, completions)
			}
			for _, want := range tt.contains {
				assert.Contains(t, completions, want)
			}
			for _, unwanted := range tt.absent {
				assert.NotContains(t, completions, unwanted)
			}
		})
	}
}

// with no snapshot loaded, commands and functions still complete but data does not
func TestCompleteWithoutData(t *testing.T) {
	st := CompletionState{
		Meta:    metaNames(),
		Funcs:   []string{"fact", "class"},
		Formats: []string{FormatHuman},
	}
	_, completions, _ := Complete(st, "(fa", 3)
	assert.Contains(t, completions, "fact ")
	_, completions, _ = Complete(st, `(fact "vi`, 9)
	assert.Empty(t, completions)
	_, completions, _ = Complete(st, ":cl", 3)
	assert.Contains(t, completions, "class ")
}

func TestCompleteRebuildsLine(t *testing.T) {
	st := testCompletionState(t)
	line := `(== (fact "virt`
	head, completions, tail := Complete(st, line, len(line))
	require.NotEmpty(t, completions)
	assert.Equal(t, `(== (fact "virtual"`, head+completions[0]+tail)
	assert.True(t, strings.HasPrefix(head+completions[0], line))
}
