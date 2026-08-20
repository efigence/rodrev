package repl

import (
	"strings"
	"testing"

	"github.com/efigence/rodrev/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyntaxHelpExamples runs every example from :syntax against the t-data node,
// so the built-in documentation can not drift away from what the query engine does
func TestSyntaxHelpExamples(t *testing.T) {
	s, _ := testSession(t, FormatHuman)
	examples := extractExpressions(Syntax())
	require.NotEmpty(t, examples)
	t.Logf("%d examples in :syntax", len(examples))
	for _, expr := range examples {
		t.Run(expr, func(t *testing.T) {
			require.NoError(t, query.CheckSyntax(expr))
			b, ok := s.backend.(*LocalBackend)
			require.True(t, ok)
			matched, err := b.Harness().Query(expr)
			require.NoError(t, err)
			assert.True(t, matched, "examples are written to match the t-data node")
		})
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	help := Help()
	for _, c := range metaCommands() {
		assert.Contains(t, help, ":"+c.Name)
		assert.Contains(t, help, c.Help)
	}
}

func TestBanner(t *testing.T) {
	s, _ := testSession(t, FormatHuman)
	banner := s.Banner()
	assert.Contains(t, banner, "local")
	assert.Contains(t, banner, "facts,")
	assert.Contains(t, banner, "classes")
	assert.Contains(t, banner, ":help")
	assert.Contains(t, banner, ":syntax")
	// the banner example must be a valid query
	require.NoError(t, query.CheckSyntax(extractExpressions(banner)[0]))
}

// extractExpressions pulls balanced lisp expressions out of a help text, ignoring
// the prose and the trailing comments after each example
func extractExpressions(text string) []string {
	out := make([]string, 0, 16)
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, " ") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "(") {
			continue
		}
		if expr, ok := balanced(trimmed); ok {
			out = append(out, expr)
		}
	}
	return out
}

// balanced returns the leading expression of s, up to the paren that closes it
func balanced(s string) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for i, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
		case r == '(':
			depth++
		case r == ')':
			depth--
			if depth == 0 {
				return s[:i+1], true
			}
		}
	}
	return "", false
}
