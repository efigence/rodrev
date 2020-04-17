package query

import (
	"fmt"
	"github.com/efigence/rodrev/common"
	"github.com/glycerine/zygomys/zygo"
	"sync"
)


type Engine struct {
	r *common.Runtime
	zg *zygo.Zlisp
	l sync.Mutex
}


func NewQueryEngine(r *common.Runtime) *Engine {
	if r == nil {
		panic("need runtime")
	}
	var e Engine
	e.zg = zygo.NewZlispSandbox()
	e.zg.ImportRegex()
	e.zg.ImportRandom()
	e.zg.AddFunction("regex", FuzzyCompareFunction)
	e.zg.AddFunction("regexp", FuzzyCompareFunction)
	e.r = r

	return &e
}


// ParseBool parses query and returns true if return is true or nonempty string, or > 0 numeric value
func (e *Engine) ParseBool(q string) (bool,error) {
	vars := e.r.Cfg.NodeMeta
	e.l.Lock()
	defer e.l.Unlock()
	e.zg.Clear()
	varsLisp, err := zygo.GoToSexp(vars, e.zg)
	if err != nil {
		return false, err
	}
	e.zg.AddGlobal("node", varsLisp)
	err = e.zg.LoadString(q)
	if err != nil {
		return false, fmt.Errorf("error parsing query [%s]: %s", q, err)
	}
	iters := 0
	e.zg.AddPreHook(
		func(zg *zygo.Zlisp, s string, se []zygo.Sexp) {
			iters++
			if iters > 1000 {
				zg.Clear()
			}
		})
	expr, err := e.zg.Run()
	e.zg.Clear()
	if iters >= 1000 {
		return false, fmt.Errorf("query [%s]: iterations limit exceeded: %d", q, iters)
	}
	if err != nil {
		return false, fmt.Errorf("error running query [%s]: %s", q, err)
	}
	switch v := expr.(type) {
	case *zygo.SexpBool:
		return v.Val, nil
	case *zygo.SexpStr:
		if len(v.S) > 0 {
			return true, nil
		} else {
			return false, nil
		}
	case *zygo.SexpInt:
		if v.Val > 0 {
			return true, nil
		} else {
			return false, nil
		}
	default:
		return false, fmt.Errorf("query return type %+v[%T] not supported, make your query return bool (or string/int > 0)",expr,expr)
	}
}
