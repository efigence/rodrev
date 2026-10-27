package query

import (
	"github.com/glycerine/zygomys/v6/zygo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	"testing"
)

var yt1 = `
---
string_key: s1
string_arr:
  - s2
  - s3
string_hash:
  str1: s4
  str2: s5
string_hash_mix:
  str1: s6
  int1: 1
  float1: 0.1
nested_hash_mix:
  str1:
    str2:
      str3: s7
      float3: 0.3
nested_hash_array_mix:
  str1:
    arr1:
      - { str1: s8 } 
      - { str2: s9 } 
`

func Test_traverseHash(t *testing.T) {
	var f map[string]interface{}
	err := yaml.Unmarshal([]byte(yt1), &f)
	require.NoError(t, err)
	t.Run("string key", func(t *testing.T) {
		v := traverseHash([]string{"string_key"}, f)
		assert.IsType(t, string(""), v)
		assert.Equal(t, "s1", v)
	})
	t.Run("string arr", func(t *testing.T) {
		v := traverseHash([]string{"string_arr"}, f)
		require.IsType(t, make([]interface{}, 0), v)
		va, _ := v.([]interface{})
		require.Len(t, va, 2)
		assert.Equal(t, "s2", va[0])
		assert.Equal(t, "s3", va[1])
	})
	t.Run("string hash", func(t *testing.T) {
		v := traverseHash([]string{"string_hash"}, f)
		require.IsType(t, make(map[string]interface{}, 0), v)
		vh, _ := v.(map[string]interface{})
		require.Len(t, vh, 2)
		assert.Equal(t, "s4", vh["str1"])
		assert.Equal(t, "s5", vh["str2"])
	})
	t.Run("string hash mix", func(t *testing.T) {
		v := traverseHash([]string{"string_hash_mix"}, f)
		require.IsType(t, make(map[string]interface{}, 0), v)
		vh, _ := v.(map[string]interface{})
		require.Len(t, vh, 3)
		assert.Equal(t, "s6", vh["str1"])
		assert.Equal(t, 1, vh["int1"])
		assert.Equal(t, 0.1, vh["float1"])
	})
	t.Run("nested hash mix", func(t *testing.T) {
		v1 := traverseHash([]string{"nested_hash_mix", "str1", "str2", "str3"}, f)
		assert.IsType(t, string(""), v1)
		v2 := traverseHash([]string{"nested_hash_mix", "str1", "str2", "float3"}, f)
		assert.IsType(t, float64(0), v2)

	})
	t.Run("nested hash array mix", func(t *testing.T) {
		v1 := traverseHash([]string{"nested_hash_array_mix", "str1", "arr1", "9"}, f)
		assert.Equal(t, nil, v1)
		v2 := traverseHash([]string{"nested_hash_array_mix", "str1", "arr1", "1", "str2"}, f)
		assert.Equal(t, string("s9"), v2)
	})
	t.Run("array element", func(t *testing.T) {
		assert.Equal(t, "s3", traverseHash([]string{"string_arr", "1"}, f))
	})
	// used to index the slice with -1 and panic
	t.Run("negative array index", func(t *testing.T) {
		assert.Nil(t, traverseHash([]string{"string_arr", "-1"}, f))
	})
	t.Run("non-numeric array index", func(t *testing.T) {
		assert.Nil(t, traverseHash([]string{"string_arr", "first"}, f))
	})
	t.Run("missing key", func(t *testing.T) {
		assert.Nil(t, traverseHash([]string{"string_hash", "str9"}, f))
	})
	t.Run("past a leaf", func(t *testing.T) {
		assert.Nil(t, traverseHash([]string{"string_key", "deeper"}, f))
		assert.Nil(t, traverseHash([]string{"string_arr", "0", "deeper"}, f))
	})
	t.Run("unsupported type", func(t *testing.T) {
		assert.Nil(t, traverseHash([]string{"flag", "deeper"}, map[string]interface{}{"flag": true}))
	})

}

func TestFuzzyCompareFunction(t *testing.T) {
	str := func(s string) *zygo.SexpStr { return &zygo.SexpStr{S: s} }
	tests := []struct {
		name    string
		fn      string
		args    []zygo.Sexp
		want    bool
		wantErr bool
	}{
		{name: "string match", fn: "regexp", args: []zygo.Sexp{str("kvm"), str("^k")}, want: true},
		{name: "string mismatch", fn: "regex", args: []zygo.Sexp{str("kvm"), str("^v")}, want: false},
		{name: "int is matched as its decimal form", fn: "regexp", args: []zygo.Sexp{&zygo.SexpInt{Val: 42}, str("^4")}, want: true},
		{name: "wrong number of args", fn: "regexp", args: []zygo.Sexp{str("kvm")}, wantErr: true},
		{name: "first arg not string or int", fn: "regexp", args: []zygo.Sexp{&zygo.SexpBool{Val: true}, str("x")}, wantErr: true},
		{name: "regexp not a string", fn: "regexp", args: []zygo.Sexp{str("kvm"), &zygo.SexpInt{Val: 1}}, wantErr: true},
		{name: "regexp does not compile", fn: "regexp", args: []zygo.Sexp{str("kvm"), str("(")}, wantErr: true},
		{name: "unknown function name", fn: "glob", args: []zygo.Sexp{str("kvm"), str("k*")}, wantErr: true},
		// was once meant to be a negated match, but never registered. Use (not (regexp ...))
		{name: "rfr/ is not a function", fn: "rfr/", args: []zygo.Sexp{str("kvm"), str("^k")}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := FuzzyCompareFunction(nil, tt.fn, tt.args)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.IsType(t, &zygo.SexpBool{}, out)
			assert.Equal(t, tt.want, out.(*zygo.SexpBool).Val)
		})
	}
}

func Test_sexpArrToStr(t *testing.T) {
	in := &zygo.SexpArray{Val: []zygo.Sexp{
		&zygo.SexpStr{S: "os"},
		&zygo.SexpArray{Val: []zygo.Sexp{&zygo.SexpStr{S: "release"}, &zygo.SexpInt{Val: 0}}},
	}}
	assert.Equal(t, []string{"os", "release", "0"}, sexpArrToStr(in))
	// a wrong type turns into a path element that cannot match, rather than an error
	out := sexpArrToStr(&zygo.SexpArray{Val: []zygo.Sexp{&zygo.SexpBool{Val: true}}})
	require.Len(t, out, 1)
	assert.Contains(t, out[0], "wrong type")
}
