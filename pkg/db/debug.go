package db

import (
	"fmt"
	"sync/atomic"
)

var debugModeEnabled atomic.Bool

// SetDebugMode toggles debug-only prints in pkg/db.
func SetDebugMode(enabled bool) {
	debugModeEnabled.Store(enabled)
}

func debugPrintln(a ...interface{}) {
	if !debugModeEnabled.Load() {
		return
	}
	fmt.Println(a...)
}

func debugPrintf(format string, a ...interface{}) {
	if !debugModeEnabled.Load() {
		return
	}
	fmt.Printf(format, a...)
}
