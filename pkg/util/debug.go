package util

import (
	"fmt"
	"sync/atomic"
)

var debugModeEnabled atomic.Bool

// SetDebugMode toggles debug-only prints in pkg/util.
func SetDebugMode(enabled bool) {
	debugModeEnabled.Store(enabled)
}

func debugPrintln(a ...interface{}) {
	if !debugModeEnabled.Load() {
		return
	}
	fmt.Println(a...)
}
