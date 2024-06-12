package util

import (
	"fmt"
	"time"
)

func StringOrPanic(s string, err error) string {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return s
}

func BoolOrPanic(b bool, err error) bool {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return b
}
func DurationOrPanic(d time.Duration, err error) time.Duration {
	if err != nil {
		panic(fmt.Sprintf("error getting argument: %s", err))
	}
	return d
}
