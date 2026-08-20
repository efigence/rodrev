package repl

import (
	"os"
	"path/filepath"
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

// the line editor wants candidates with the typed prefix stripped off, plus how
// many runes they replace
func TestCompleteRunes(t *testing.T) {
	st := testCompletionState(t)
	tests := []struct {
		name     string
		line     string
		typed    int
		contains []string
	}{
		{name: "function", line: "(fa", typed: 2, contains: []string{"ct "}},
		{name: "meta command", line: ":cl", typed: 2, contains: []string{"ass ", "uster "}},
		{name: "fact name in string", line: `(== (fact "virt`, typed: 4, contains: []string{`ual"`}},
		{name: "nested fact path", line: `(fact "os" "distro" "co`, typed: 2, contains: []string{`dename"`}},
		{name: "class segment", line: `(class "mon::che`, typed: 8, contains: []string{"ck::"}},
		{name: "nothing to complete", line: `(fact "nosuchfactname`, typed: 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runes := []rune(tt.line)
			got, length := completeRunes(st, runes, len(runes))
			assert.Equal(t, tt.typed, length, "replaced prefix length")
			as := make([]string, 0, len(got))
			for _, c := range got {
				as = append(as, string(c))
			}
			for _, want := range tt.contains {
				assert.Contains(t, as, want)
			}
			if len(tt.contains) == 0 {
				assert.Empty(t, as)
			}
			// the typed prefix plus a candidate rebuilds a valid line
			for _, c := range got {
				assert.True(t, len(tt.line)+len(string(c)) > len(tt.line))
			}
		})
	}
}

func TestCompleteSnapshotSubcommands(t *testing.T) {
	st := testCompletionState(t)
	st.Nodes = []string{"d1-lg.example.com", "d2-lg.example.com"}
	st.Files = func(prefix string) []string { return []string{prefix + "node.json "} }
	tests := []struct {
		name     string
		line     string
		head     string
		contains []string
	}{
		{name: "nodes and subcommands", line: ":snapshot ", head: ":snapshot ",
			contains: []string{"save ", "load ", "d1-lg.example.com "}},
		{name: "narrowed to a subcommand", line: ":snapshot sa", head: ":snapshot ",
			contains: []string{"save "}},
		{name: "path after save", line: ":snapshot save prod", head: ":snapshot save ",
			contains: []string{"prodnode.json "}},
		{name: "path after load", line: ":snapshot load ", head: ":snapshot load ",
			contains: []string{"node.json "}},
		{name: "path for local", line: ":local t-", head: ":local ",
			contains: []string{"t-node.json "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head, completions, _ := Complete(st, tt.line, len([]rune(tt.line)))
			assert.Equal(t, tt.head, head)
			for _, want := range tt.contains {
				assert.Contains(t, completions, want)
			}
		})
	}
}

// with no path completer wired up, path arguments simply do not complete
func TestCompleteNoPathCompleter(t *testing.T) {
	st := testCompletionState(t)
	st.Files = nil
	_, completions, _ := Complete(st, ":snapshot save x", len(":snapshot save x"))
	assert.Empty(t, completions)
}

func TestGlobPaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node.json"), []byte("{}"), 0600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0700))
	got := GlobPaths(dir + string(os.PathSeparator))
	assert.Contains(t, got, filepath.Join(dir, "node.json")+" ")
	// directories get a separator so the next segment can be typed straight away
	assert.Contains(t, got, filepath.Join(dir, "sub")+string(os.PathSeparator))
}
