package repl

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// streamResult prints a node result as it arrives, so a slow fleet shows progress.
// Only human output streams; csv/json are written once the query is done
func (s *Session) streamResult(r NodeResult) {
	if s.format != FormatHuman || !s.stream {
		return
	}
	switch {
	case len(r.Err) > 0:
		// errors are collected and collapsed in printQuery instead: a bad query
		// makes every node answer with the same message
		return
	case r.Matched:
	case s.verbose:
	default:
		return
	}
	if s.streamed >= streamLimit && !s.verbose {
		s.suppressed++
		return
	}
	s.streamed++
	if r.Matched {
		s.printf("  + %s", r.FQDN)
	} else {
		s.printf("  - %s", r.FQDN)
	}
}

func (s *Session) printQuery(sum Summary, results []NodeResult) error {
	switch s.format {
	case FormatCSV:
		return s.printQueryCSV(sum, results)
	case FormatJSON:
		return s.printQueryJSON(sum, results)
	default:
		return s.printQueryHuman(sum, results)
	}
}

func (s *Session) printQueryHuman(sum Summary, results []NodeResult) error {
	for _, line := range collapseErrors(results) {
		s.printf("  ! %s", line)
	}
	// a single-node result is more useful as a value than as a count
	if sum.Known == 1 && sum.Responded == 1 && len(results) == 1 {
		r := results[0]
		if len(r.Err) > 0 {
			return nil
		}
		if sum.Boolish {
			s.printf("=> %v", r.Matched)
			return nil
		}
		value := strings.TrimSpace(sum.Value)
		if strings.Contains(value, "\n") {
			s.printf("=>\n%s", indent(value, "   "))
		} else {
			s.printf("=> %s", value)
		}
		s.printf("   (%s, not a boolean - this query would not match anything)", sum.Type)
		return nil
	}
	total := ""
	if sum.Known > 0 {
		total = "/" + strconv.Itoa(sum.Known)
	}
	errs := ""
	if sum.Errors > 0 {
		errs = fmt.Sprintf(", %d errors", sum.Errors)
	}
	answered := fmt.Sprintf(", %d answered", sum.Responded)
	if sum.Known > 0 && sum.Responded < sum.Known {
		answered = fmt.Sprintf(", %d of %d answered", sum.Responded, sum.Known)
	}
	if s.suppressed > 0 {
		s.printf("  ... %d more not listed (:verbose to list every node)", s.suppressed)
	}
	s.printf("%d%s matched%s%s, %s",
		sum.Matched, total, answered, errs, sum.Elapsed.Round(time.Millisecond))
	switch {
	case sum.Responded == 0 && sum.Known == 0:
		s.printf("   no node answered and none were discovered - is the MQ reachable? (:nodes -r)")
	case sum.Silent && sum.Responded < sum.Known:
		// only the daemons that answer no-match can be counted, so the rest of
		// the fleet is unaccounted for rather than known to be down
		s.printf("   nodes that did not match stayed silent, so only the match count is exact")
	case sum.Known > 0 && sum.Responded < sum.Known:
		s.printf("   %d nodes did not answer within %s (:timeout to wait longer)",
			sum.Known-sum.Responded, s.timeout)
	}
	return nil
}

// collapseErrors groups identical node errors: a query that fails to evaluate
// fails the same way on every node, and nobody wants 1000 copies of it
func collapseErrors(results []NodeResult) []string {
	byErr := make(map[string][]string, 4)
	order := make([]string, 0, 4)
	for _, r := range results {
		if len(r.Err) == 0 {
			continue
		}
		if _, ok := byErr[r.Err]; !ok {
			order = append(order, r.Err)
		}
		byErr[r.Err] = append(byErr[r.Err], r.FQDN)
	}
	out := make([]string, 0, len(order))
	for _, err := range order {
		nodes := byErr[err]
		switch len(nodes) {
		case 1:
			out = append(out, fmt.Sprintf("%s: %s", nodes[0], err))
		case 2, 3:
			out = append(out, fmt.Sprintf("%s: %s", strings.Join(nodes, " "), err))
		default:
			out = append(out, fmt.Sprintf("%d nodes (%s ...): %s", len(nodes), nodes[0], err))
		}
	}
	return out
}

func (s *Session) printQueryCSV(sum Summary, results []NodeResult) error {
	w := csv.NewWriter(s.out)
	defer w.Flush()
	if !s.csvHeader["query"] {
		s.csvHeader["query"] = true
		if err := w.Write([]string{"expr", "fqdn", "matched", "error"}); err != nil {
			return err
		}
	}
	for _, r := range results {
		matched := "0"
		if r.Matched {
			matched = "1"
		}
		if err := w.Write([]string{sum.Expr, r.FQDN, matched, r.Err}); err != nil {
			return err
		}
	}
	return w.Error()
}

// queryJSON is one NDJSON record per query, so multiple -e flags stay streamable
type queryJSON struct {
	Expr      string            `json:"expr"`
	ElapsedMs int64             `json:"elapsed_ms"`
	Matched   []string          `json:"matched"`
	Unmatched []string          `json:"unmatched"`
	Errors    map[string]string `json:"errors,omitempty"`
	Summary   summaryJSON       `json:"summary"`
	Type      string            `json:"type,omitempty"`
	Value     string            `json:"value,omitempty"`
}

type summaryJSON struct {
	Matched   int `json:"matched"`
	Responded int `json:"responded"`
	Known     int `json:"known"`
	Errors    int `json:"errors"`
}

func (s *Session) printQueryJSON(sum Summary, results []NodeResult) error {
	out := queryJSON{
		Expr:      sum.Expr,
		ElapsedMs: sum.Elapsed.Milliseconds(),
		Matched:   make([]string, 0, len(results)),
		Unmatched: make([]string, 0, len(results)),
		Summary: summaryJSON{
			Matched:   sum.Matched,
			Responded: sum.Responded,
			Known:     sum.Known,
			Errors:    sum.Errors,
		},
		Type:  sum.Type,
		Value: sum.Value,
	}
	for _, r := range results {
		switch {
		case len(r.Err) > 0:
			if out.Errors == nil {
				out.Errors = make(map[string]string, 1)
			}
			out.Errors[r.FQDN] = r.Err
		case r.Matched:
			out.Matched = append(out.Matched, r.FQDN)
		default:
			out.Unmatched = append(out.Unmatched, r.FQDN)
		}
	}
	sort.Strings(out.Matched)
	sort.Strings(out.Unmatched)
	enc := json.NewEncoder(s.out)
	return enc.Encode(&out)
}

// printFacts renders fact values in the current output format
func (s *Session) printFacts(facts map[string]interface{}) error {
	names := make([]string, 0, len(facts))
	for name := range facts {
		names = append(names, name)
	}
	sort.Strings(names)
	switch s.format {
	case FormatCSV:
		w := csv.NewWriter(s.out)
		defer w.Flush()
		if !s.csvHeader["fact"] {
			s.csvHeader["fact"] = true
			if err := w.Write([]string{"fqdn", "fact", "value"}); err != nil {
				return err
			}
		}
		for _, name := range names {
			if err := w.Write([]string{s.snapshotFQDN(), name, renderValue(facts[name])}); err != nil {
				return err
			}
		}
		return w.Error()
	case FormatJSON:
		return json.NewEncoder(s.out).Encode(map[string]interface{}{
			s.snapshotFQDN(): facts,
		})
	}
	for _, name := range names {
		value := renderValue(facts[name])
		if strings.Contains(value, "\n") {
			s.printf("%s:\n%s", name, indent(value, "  "))
		} else {
			s.printf("%s: %s", name, value)
		}
	}
	return nil
}

func (s *Session) printClasses(classes []string) error {
	switch s.format {
	case FormatCSV:
		w := csv.NewWriter(s.out)
		defer w.Flush()
		if !s.csvHeader["class"] {
			s.csvHeader["class"] = true
			if err := w.Write([]string{"fqdn", "class"}); err != nil {
				return err
			}
		}
		for _, class := range classes {
			if err := w.Write([]string{s.snapshotFQDN(), class}); err != nil {
				return err
			}
		}
		return w.Error()
	case FormatJSON:
		return json.NewEncoder(s.out).Encode(map[string][]string{s.snapshotFQDN(): classes})
	}
	for _, class := range classes {
		s.printf("  %s", class)
	}
	s.printf("%d classes", len(classes))
	return nil
}

func (s *Session) snapshotFQDN() string {
	if s.snapshot == nil {
		return ""
	}
	return s.snapshot.FQDN
}
