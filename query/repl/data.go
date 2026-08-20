package repl

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// factPath walks a fact map the same way the `fact` query function does:
// map keys, and integer indexes into arrays
func factPath(facts map[string]interface{}, keys []string) (interface{}, bool) {
	var current interface{} = facts
	for _, key := range keys {
		switch v := current.(type) {
		case map[string]interface{}:
			next, ok := v[key]
			if !ok {
				return nil, false
			}
			current = next
		case []interface{}:
			idx, err := strconv.Atoi(key)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			current = v[idx]
		default:
			return nil, false
		}
	}
	return current, true
}

// factChildren returns names of the entries directly below keys. Array elements
// are returned as their indexes, so they can be used as a path segment
func factChildren(facts map[string]interface{}, keys []string) []string {
	node, ok := factPath(facts, keys)
	if !ok {
		return nil
	}
	switch v := node.(type) {
	case map[string]interface{}:
		out := make([]string, 0, len(v))
		for k := range v {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for i := range v {
			out = append(out, strconv.Itoa(i))
		}
		return out
	}
	return nil
}

// matchNames filters names by a glob pattern; a pattern with no glob character
// matches by prefix, so `:fact apt` and `:fact apt_*` both do what one expects
func matchNames(names []string, pattern string) []string {
	if len(pattern) == 0 {
		return names
	}
	glob := strings.ContainsAny(pattern, "*?[")
	out := make([]string, 0, len(names))
	for _, name := range names {
		if glob {
			if ok, err := path.Match(pattern, name); err == nil && ok {
				out = append(out, name)
			}
		} else if strings.HasPrefix(name, pattern) {
			out = append(out, name)
		}
	}
	return out
}

// renderValue renders a fact value: scalars as-is, structures as YAML
func renderValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "(not set)"
	case string:
		return val
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", val)
	}
	out, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return strings.TrimRight(string(out), "\n")
}

// indent shifts every line of a multiline value, so nested values stay readable
func indent(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
