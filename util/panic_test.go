package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// these wrap flag lookups, where an error means the flag was never defined -
// a programming mistake, not something a user can cause
func TestOrPanic(t *testing.T) {
	assert.Equal(t, "value", StringOrPanic("value", nil))
	assert.Equal(t, true, BoolOrPanic(true, nil))
	assert.Equal(t, time.Minute, DurationOrPanic(time.Minute, nil))

	err := fmt.Errorf("flag accessed but not defined: nosuchflag")
	assert.PanicsWithValue(t,
		"error getting argument: flag accessed but not defined: nosuchflag",
		func() { StringOrPanic("", err) })
	assert.Panics(t, func() { BoolOrPanic(false, err) })
	assert.Panics(t, func() { DurationOrPanic(0, err) })
}
