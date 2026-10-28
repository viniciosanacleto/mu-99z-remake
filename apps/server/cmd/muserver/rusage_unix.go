//go:build !windows

package main

import (
	"runtime"
	"syscall"
)

// processUsage returns the CPU time of the process and its peak resident
// set size in bytes.
func processUsage() (user, sys float64, maxRSS int64) {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return 0, 0, 0
	}
	maxRSS = int64(ru.Maxrss)
	if runtime.GOOS != "darwin" { // Linux and the BSDs report kilobytes
		maxRSS *= 1024
	}
	return tvSeconds(ru.Utime), tvSeconds(ru.Stime), maxRSS
}

func tvSeconds(tv syscall.Timeval) float64 {
	return float64(tv.Sec) + float64(tv.Usec)/1e6
}
