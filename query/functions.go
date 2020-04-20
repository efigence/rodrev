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

func sexpArrToStr(args *zygo.SexpArray) []string {
	out := make([]string,0)
	for _, arg := range args.Val {
		switch v := arg.(type) {
		case *zygo.SexpArray:
			out = append(out, sexpArrToStr(v)...)
		case *zygo.SexpStr:
			out = append(out,v.S)
		case *zygo.SexpInt:
			out = append(out,strconv.Itoa(int(v.Val)))
		default:
			out = append(out,fmt.Sprintf("wrong type %T in input %+v",v, args))
		}
	}
	return out
}


func HashGet(hash *map[string]interface{}) func (env *zygo.Zlisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {

	return func(env *zygo.Zlisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
		if len(args) == 0 {
			return zygo.SexpNull, zygo.WrongNargs
		}

		path := make ([]string,0)
		for _, arg := range args {
			switch v := arg.(type) {
			case *zygo.SexpStr:
				path = append(path,v.S)
			case *zygo.SexpArray:
				path = append(path, sexpArrToStr(v)...)
			case *zygo.SexpInt:
				path = append(path,strconv.Itoa(int(v.Val)))
			default:
				return zygo.SexpNull,fmt.Errorf("wrong argument type %T in the list %+v",v,args)
			}
		}
		if len(path) == 0 {
			return zygo.SexpNull, fmt.Errorf("resulting parse got 0 elements: %+v", args)
		}

		val := traverseHash(path,*hash)
		out, err := zygo.GoToSexp(val,env)
		if err != nil {
			return zygo.SexpNull,fmt.Errorf("error converting [%+v] to Sexp: %s",out,err)

		}
		return out,nil

	}
}