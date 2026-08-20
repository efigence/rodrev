package query

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"github.com/glycerine/zygomys/zygo"
	"sort"
)

type Engine struct {
	r       *common.Runtime
	dataMap map[string]MapGetter
}

func NewQueryEngine(r *common.Runtime) *Engine {
	if r == nil {
		panic("need runtime")
	}
	return &Engine{
		r:       r,
		dataMap: make(map[string]MapGetter, 0),
	}
}

// MapGetter returns reference to a map
type MapGetter interface {
	Map() *map[string]interface{}
}

func (e *Engine) RegisterMap(name string, m MapGetter) error {
	e.dataMap[name] = m
	return nil

}

// DataMaps returns names of the registered data functions (like "fact" or "class")
func (e *Engine) DataMaps() []string {
	out := make([]string, 0, len(e.dataMap))
	for name := range e.dataMap {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Result is an outcome of running a query, before it is coerced to a boolean
type Result struct {
	// Bool is the coerced value. False whenever Boolish is false
	Bool bool
	// Boolish is set when query returned something that can be used as a filter:
	// a bool, a non-empty string or an integer
	Boolish bool
	// Type is a human readable type of returned value: bool, string, int, hash, ...
	Type string
	// Display is the returned value rendered for display
	Display string
	raw     zygo.Sexp
}

// newSandbox returns zygo environment with rodrev functions and data registered.
// Caller MUST call Stop() on it, else it WILL leak memory - it uses goroutines underneath
func (e *Engine) newSandbox() *zygo.Zlisp {
	zg := zygo.NewZlispSandbox()
	zg.ImportRegex()
	zg.ImportRandom()
	zg.AddFunction("regex", FuzzyCompareFunction)
	zg.AddFunction("regexp", FuzzyCompareFunction)
	for n, m := range e.dataMap {
		zg.AddFunction(n, HashGet(m.Map()))
	}
	return zg
}

// Parse runs the query and returns its result without coercing it to a boolean.
// Unlike ParseBool a non-boolean result is not an error, so a query can be
// inspected (`(fact "os")` returns a hash) while it is being written
func (e *Engine) Parse(q string) (Result, error) {
	var res Result
	vars := e.r.Cfg.NodeMeta
	zg := e.newSandbox()
	defer zg.Stop() // else it WILL leak memory - it uses goroutines underneath
	varsLisp, err := zygo.GoToSexp(vars, zg)
	if err != nil {
		return res, err
	}
	zg.AddGlobal("node", varsLisp)
	err = zg.LoadString(q)
	if err != nil {
		return res, fmt.Errorf("error parsing query [%s]: %s", q, err)
	}
	iters := 0
	zg.AddPreHook(
		func(zg *zygo.Zlisp, s string, se []zygo.Sexp) {
			iters++
			if iters > 1000 {
				zg.Clear()
			}
		})
	expr, err := zg.Run()
	if iters >= 1000 {
		return res, fmt.Errorf("query [%s]: iterations limit exceeded: %d", q, iters)
	}
	if err != nil {
		return res, fmt.Errorf("error running query [%s]: %s", q, err)
	}
	return newResult(expr), nil
}

func newResult(expr zygo.Sexp) Result {
	res := Result{raw: expr}
	switch v := expr.(type) {
	case *zygo.SexpBool:
		res.Type = "bool"
		res.Boolish = true
		res.Bool = v.Val
	case *zygo.SexpStr:
		res.Type = "string"
		res.Boolish = true
		res.Bool = len(v.S) > 0
	case *zygo.SexpInt:
		res.Type = "int"
		res.Boolish = true
		res.Bool = v.Val > 0
	case *zygo.SexpFloat:
		res.Type = "float"
	case *zygo.SexpHash:
		res.Type = "hash"
	case *zygo.SexpArray:
		res.Type = "array"
	case *zygo.SexpSentinel:
		res.Type = "nil"
	default:
		res.Type = fmt.Sprintf("%T", expr)
	}
	if expr != nil {
		res.Display = expr.SexpString(nil)
	}
	return res
}

// ParseBool parses query and returns true if return is true or nonempty string, or > 0 numeric value
func (e *Engine) ParseBool(q string) (bool, error) {
	res, err := e.Parse(q)
	if err != nil {
		return false, err
	}
	if !res.Boolish {
		return false, fmt.Errorf("query return type %+v[%T] not supported, make your query return bool (or string/int > 0)", res.raw, res.raw)
	}
	return res.Bool, nil
}

// CheckSyntax parses and compiles the query without running it. It needs no data
// and no runtime, so a client can reject a malformed query before sending it to
// the whole fleet. Errors that only show up while running (a fact that is a hash
// where a string was expected) can only be reported by the node itself
func CheckSyntax(q string) error {
	zg := zygo.NewZlispSandbox()
	defer zg.Stop() // else it WILL leak memory - it uses goroutines underneath
	zg.ImportRegex()
	zg.ImportRandom()
	if err := zg.LoadString(q); err != nil {
		return fmt.Errorf("error parsing query [%s]: %s", q, err)
	}
	return nil
}
