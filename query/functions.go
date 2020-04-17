package query

import (
	"fmt"
	"github.com/glycerine/zygomys/zygo"
	"regexp"
	"strconv"
)

func FuzzyCompareFunction(env *zygo.Zlisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
	if len(args) != 2 {
		return zygo.SexpNull, zygo.WrongNargs
	}

	arg1 :=""
	switch v := args[0].(type) {
	case *zygo.SexpInt:
		arg1 = strconv.Itoa(int(v.Val))
	case *zygo.SexpStr:
		arg1 = v.S
	default:
		return zygo.SexpNull, fmt.Errorf("%s accepts string/int as first argument, not %T",name,args[0])
	}
	arg2 :=""
	switch v := args[1].(type) {
	case *zygo.SexpStr:
		arg2 = v.S
	default:
		return zygo.SexpNull, fmt.Errorf("%s accepts string as second argument, not %T",name,args[1])
	}
	reverse := false
	switch name {
	case "regexp":
		reverse = false
	case "regex":
		reverse = false
	case "rfr/":
		reverse = true

	default:
		return zygo.SexpNull, fmt.Errorf("%s is not a supported function",name)
	}
	re, err := regexp.Compile(arg2)
	if err != nil {
		return zygo.SexpNull, fmt.Errorf("compiling regexp [%s] failed: %s", arg2, err)
	}
	cond :=  re.MatchString(arg1)
	if reverse {
		cond = !cond
	}
	return &zygo.SexpBool{Val: cond}, nil
}