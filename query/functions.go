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

// traverse map[string]interface{} hashes made by yaml-like unmarshallers
func traverseHash(args []string, hash map[string]interface{}) interface{} {
	var root interface{}
	root = hash
	for idx, arg := range args {
		_ = arg
		switch v := root.(type) {
		case map[string]interface{}:
			if v, ok := v[arg];ok {
				root = v
			} else {
				return nil
			}
		case []interface{}:
			iarg, err := strconv.Atoi(arg)
			if err != nil {return nil}
			if iarg + 1  > len(v)  {
				return nil
			} else {
				if idx == len(args)-1 {
					return v[iarg]
				} else {
					root = v[iarg]
				}
			}
		case string,int,int8,int16,int32,int64,uint,uint8,uint16,uint32,uint64,float32,float64,[]byte:
			if idx == len(args) - 1 {
				return v
			} else {
				return nil
			}
		default: return nil
		}
	}
	return root
}


func HashGet(hash *map[string]interface{}) func (env *zygo.Zlisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {

	return func(env *zygo.Zlisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
		if len(args) == 0 {
			return zygo.SexpNull, zygo.WrongNargs
		}
		root := *hash
		// for idx, arg := range args {
		// 	switch v := root.(type) {
		// 	case map[string]interface{}:
		//
		// 	}
		// 	_ = idx
		// 	_ = arg
		// }
		_ = root
		key := ""
		_ = key
		switch v := args[0].(type) {
		case *zygo.SexpStr:
			_ = v
			//arg1 = v.S
		default:
			return zygo.SexpNull, fmt.Errorf("%s accepts string/int as first argument, not %T", name, args[0])
		}
		arg2 := ""
		switch v := args[1].(type) {
		case *zygo.SexpStr:
			arg2 = v.S
		default:
			return zygo.SexpNull, fmt.Errorf("%s accepts string as second argument, not %T", name, args[1])
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
			return zygo.SexpNull, fmt.Errorf("%s is not a supported function", name)
		}
		re, err := regexp.Compile(arg2)
		if err != nil {
			return zygo.SexpNull, fmt.Errorf("compiling regexp [%s] failed: %s", arg2, err)
		}
		cond := re.MatchString("")
		if reverse {
			cond = !cond
		}
		return &zygo.SexpBool{Val: cond}, nil
	}
}