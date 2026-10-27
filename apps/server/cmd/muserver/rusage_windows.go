package main

// processUsage is not sampled on Windows; cmd/muload reads the process
// from outside instead.
func processUsage() (user, sys float64, maxRSS int64) { return 0, 0, 0 }
