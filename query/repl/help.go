package repl

import (
	"fmt"
	"strings"
)

// syntaxHelp is the full query language reference shown by :syntax. Every example
// in here is parsed and evaluated against t-data by TestSyntaxHelpExamples, so it
// can not go stale
const syntaxHelp = `Queries are zygomys lisp, evaluated on each node; result must be a boolean
(a non-empty string or an int > 0 also count as true).

fact - facter data, from facts.yaml
  (== (fact "virtual") "kvm")                        flat fact
  (== (fact "os" "distro" "codename") "bookworm")    nested: one argument per level
  (== (fact "processors" "count") 4)                 numbers compare with == < >
  (fact "is_virtual")                                a boolean fact is a query by itself
  (== (fact "networking" "interfaces" "eth0" "bindings" 0 "address") "10.100.101.33")
                                                     ints index into arrays

class - classes applied by the last puppet run, from classes.txt
  (== (class "nginx") true)                          class is present
  (!= (class "apache") true)                         class is absent

node - node_meta from the daemon config file, not facter
  (== (-> node %fqdn) "d1-lg.example.com")
  (regexp (-> node %fqdn) "^d1-")                    regexp match
  (regexp (-> node %certname) "example.com$")

combine with and / or / not
  (and (== (fact "virtual") "kvm") (== (class "nginx") true))
  (or (== (class "apache") true) (== (class "nginx") true))
  (not (== (class "apache") true))

Inspect data instead of matching it:
  :fact os.distro       show a fact value      :fact apt_*    glob fact names
  :class mon::*         grep classes           :nodes         list nodes
Full language reference: https://github.com/glycerine/zygomys/wiki/Language`

// Syntax returns the query language reference
func Syntax() string { return syntaxHelp }

// Help returns the meta command list
func Help() string {
	var b strings.Builder
	b.WriteString("commands:\n")
	for _, c := range metaCommands() {
		name := ":" + c.Name
		if len(c.Args) > 0 {
			name += " " + c.Args
		}
		fmt.Fprintf(&b, "  %-24s %s\n", name, c.Help)
	}
	b.WriteString("\nanything else is evaluated as a query. :syntax shows the query language,\n")
	b.WriteString("TAB completes commands, functions, fact and class names\n")
	return b.String()
}

// Banner is the short help shown when the REPL starts
func (s *Session) Banner() string {
	var b strings.Builder
	nodes := ""
	if n := len(s.nodes); n > 0 {
		nodes = fmt.Sprintf(", %d nodes", n)
	}
	if s.dataSummary() != "" {
		nodes = ", " + s.dataSummary()
	}
	fmt.Fprintf(&b, "rv query - %s%s\n", s.backend.Describe(), nodes)
	b.WriteString("  (== (class \"nginx\") true)          which nodes have that class\n")
	b.WriteString("  (== (fact \"virtual\") \"kvm\")        which nodes have that fact value\n")
	b.WriteString(":help for commands, :syntax for the query language, TAB completes\n")
	return b.String()
}

// dataSummary describes the loaded fact/class data, if any
func (s *Session) dataSummary() string {
	if s.snapshot == nil {
		return ""
	}
	return fmt.Sprintf("%d facts, %d classes", len(s.snapshot.Facts), len(s.snapshot.Classes))
}
