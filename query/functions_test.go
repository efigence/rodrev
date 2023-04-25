package query

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

}
