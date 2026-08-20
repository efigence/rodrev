package repl

import (
	"sort"
	"strings"
)

// CompletionState is everything the completer knows. It is a plain struct so
// completion can be tested without a terminal or a cluster
type CompletionState struct {
	Meta     []string
	Funcs    []string
	Facts    map[string]interface{}
	Classes  []string
	Nodes    []string
	NodeMeta map[string]interface{}
	Formats  []string
}

// tokenBreak are the characters that end a symbol in a query
const tokenBreak = " \t()\"'"

// Complete completes the word under the cursor. The result follows liner's
// WordCompleter contract: the new line is head + completion + tail
func Complete(st CompletionState, line string, pos int) (string, []string, string) {
	runes := []rune(line)
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	left := string(runes[:pos])
	tail := string(runes[pos:])
	if strings.HasPrefix(strings.TrimLeft(left, " \t"), ":") {
		head, completions := completeMeta(st, left)
		return head, completions, tail
	}
	head, completions := completeQuery(st, left)
	return head, completions, tail
}

// completeMeta completes a meta command and its arguments
func completeMeta(st CompletionState, left string) (string, []string) {
	colon := strings.Index(left, ":")
	rest := left[colon+1:]
	// still typing the command itself
	if !strings.ContainsAny(rest, " \t") {
		return left[:colon+1], spaceAll(withPrefix(st.Meta, rest))
	}
	fields := strings.Fields(rest)
	cmd, _, ok := lookupMeta(fields[0])
	if !ok {
		return left, nil
	}
	partial := ""
	if !endsWithSpace(left) && len(fields) > 1 {
		partial = fields[len(fields)-1]
	}
	head := left[:len(left)-len(partial)]
	switch cmd.Name {
	case "snapshot":
		return head, spaceAll(withPrefix(st.Nodes, partial))
	case "out":
		return head, spaceAll(withPrefix(st.Formats, partial))
	case "fact":
		return head, spaceAll(withPrefix(factChildren(st.Facts, nil), partial))
	case "class":
		return head, spaceAll(withPrefix(st.Classes, partial))
	}
	return head, nil
}

// completeQuery completes inside a query expression
func completeQuery(st CompletionState, left string) (string, []string) {
	if start, inside := stringStart(left); inside {
		partial := left[start+1:]
		head := left[:start+1]
		fn, args := enclosingCall(left[:start])
		switch fn {
		case "fact":
			return head, quoteAll(withPrefix(factChildren(st.Facts, args), partial))
		case "class":
			return head, completeClass(st.Classes, partial)
		}
		return head, nil
	}
	start := len(left)
	for start > 0 && !strings.ContainsRune(tokenBreak, rune(left[start-1])) {
		start--
	}
	partial := left[start:]
	head := left[:start]
	// %key inside (-> node %fqdn)
	if strings.HasPrefix(partial, "%") {
		keys := make([]string, 0, len(st.NodeMeta))
		for k := range st.NodeMeta {
			keys = append(keys, "%"+k)
		}
		sort.Strings(keys)
		return head, spaceAll(withPrefix(keys, partial))
	}
	return head, spaceAll(withPrefix(st.Funcs, partial))
}

// completeClass completes a class name one `::` segment at a time, so a fleet
// with hundreds of classes does not dump all of them at once
func completeClass(classes []string, partial string) []string {
	seen := make(map[string]bool, 16)
	out := make([]string, 0, 16)
	for _, class := range classes {
		if !strings.HasPrefix(class, partial) {
			continue
		}
		candidate := class
		// cut at the first :: after what was typed so far
		if idx := strings.Index(class[len(partial):], "::"); idx >= 0 {
			candidate = class[:len(partial)+idx+2]
		} else {
			candidate = class + `"`
		}
		if !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
	}
	sort.Strings(out)
	return out
}

// stringStart reports whether the cursor sits inside a string literal, and where
// that literal starts
func stringStart(s string) (int, bool) {
	start := -1
	escaped := false
	for i, r := range s {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			if start < 0 {
				start = i
			} else {
				start = -1
			}
		}
	}
	return start, start >= 0
}

// enclosingCall returns the name of the function call the cursor is inside of,
// plus the string/int arguments already given to it. `(fact "os" "distro"` gives
// ("fact", ["os","distro"])
func enclosingCall(s string) (string, []string) {
	depth := 0
	open := -1
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				open = i
				i = -1
			} else {
				depth--
			}
		}
		if open >= 0 {
			break
		}
	}
	if open < 0 {
		return "", nil
	}
	fields := strings.Fields(s[open+1:])
	if len(fields) == 0 {
		return "", nil
	}
	args := make([]string, 0, len(fields)-1)
	for _, f := range fields[1:] {
		args = append(args, strings.Trim(f, `"`))
	}
	return fields[0], args
}

func withPrefix(candidates []string, prefix string) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// quoteAll closes the string literal, so a completed fact name is ready to use
func quoteAll(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c+`"`)
	}
	return out
}

// spaceAll appends a space to completions that are not string literals, so the
// next thing typed does not end up glued to the completed word
func spaceAll(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c+" ")
	}
	return out
}

func endsWithSpace(s string) bool {
	return len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t')
}

// CompletionState builds the completer input from the session
func (s *Session) CompletionState() CompletionState {
	st := CompletionState{
		Meta:    metaNames(),
		Funcs:   s.queryFuncs(),
		Nodes:   s.nodes,
		Formats: []string{FormatHuman, FormatCSV, FormatJSON},
	}
	if s.snapshot != nil {
		st.Facts = s.snapshot.Facts
		st.Classes = s.snapshot.Classes
	}
	if b, ok := s.backend.(*LocalBackend); ok {
		st.NodeMeta = b.Harness().Runtime.Cfg.NodeMeta
	}
	return st
}
