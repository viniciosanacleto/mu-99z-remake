package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// StepResult is what the bots of one step did.
type StepResult struct {
	Bots        int
	LoginOK     int64
	LoginFail   int64
	Disconnects int64
	Walks       int64
	Attacks     int64
	Kills       int64
	Deaths      int64
	PacketsIn   int64
	BytesIn     int64
	BytesOut    int64
	LoginTimes  []time.Duration
	LoginPhase  time.Duration
	SteadyPhase time.Duration
	Errors      map[string]int
}

// PhaseStats aggregates the samples of one step and phase.
type PhaseStats struct {
	N                 int
	CPUAvg, CPUP95    float64
	CPUMax            float64
	RSSAvg, RSSMax    float64 // bytes, NaN when unknown
	HeapAvg, HeapMax  float64
	SysMax            float64
	GoroutinesAvg     float64
	BotsAvg           float64 // bots actually in the world
	GCPerMin, GCPause float64 // collections per minute, pause seconds in total
}

func aggregate(samples []Sample, step int, phase string) PhaseStats {
	var cpu, rss, heap, sys, gor, bots []float64
	var gcs uint32
	var first, last *Sample
	var st PhaseStats
	for i := range samples {
		s := &samples[i]
		if s.Step != step || s.Phase != phase {
			continue
		}
		if first == nil {
			first = s
		}
		last = s
		st.N++
		bots = append(bots, float64(s.Bots))
		if s.CPUPercent >= 0 {
			cpu = append(cpu, s.CPUPercent)
		}
		if s.RSSBytes >= 0 {
			rss = append(rss, float64(s.RSSBytes))
		}
		if s.HeapInuse > 0 {
			heap = append(heap, float64(s.HeapInuse))
			sys = append(sys, float64(s.Sys))
			gor = append(gor, float64(s.Goroutines))
		}
		st.GCPause += s.GCPause
	}
	st.CPUAvg, st.CPUP95, st.CPUMax = mean(cpu), percentile(cpu, 95), maxOf(cpu)
	st.RSSAvg, st.RSSMax = mean(rss), maxOf(rss)
	st.HeapAvg, st.HeapMax = mean(heap), maxOf(heap)
	st.SysMax = maxOf(sys)
	st.GoroutinesAvg = mean(gor)
	st.BotsAvg = mean(bots)
	if first != nil && last != first && last.NumGC >= first.NumGC {
		gcs = last.NumGC - first.NumGC
		if d := last.Time.Sub(first.Time).Minutes(); d > 0 {
			st.GCPerMin = float64(gcs) / d
		}
	}
	return st
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func maxOf(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	m := v[0]
	for _, x := range v[1:] {
		m = max(m, x)
	}
	return m
}

// percentile uses the nearest-rank method.
func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	k := int(math.Ceil(p/100*float64(len(s)))) - 1
	return s[min(max(k, 0), len(s)-1)]
}

func durPercentile(v []time.Duration, p float64) time.Duration {
	f := make([]float64, len(v))
	for i, d := range v {
		f[i] = float64(d)
	}
	r := percentile(f, p)
	if math.IsNaN(r) {
		return 0
	}
	return time.Duration(r)
}

// linearFit returns the least-squares slope and intercept of y over x,
// skipping NaN points.
func linearFit(x, y []float64) (slope, intercept float64, ok bool) {
	var n, sx, sy, sxx, sxy float64
	for i := range x {
		if math.IsNaN(y[i]) {
			continue
		}
		n++
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	d := n*sxx - sx*sx
	if n < 2 || d == 0 {
		return 0, 0, false
	}
	slope = (n*sxy - sx*sy) / d
	return slope, (sy - slope*sx) / n, true
}

var csvHeader = []string{"time", "step", "phase", "bots", "users", "cpu_percent", "rss_bytes", "heap_inuse", "sys",
	"goroutines", "num_gc", "gc_pause_s"}

func writeSampleCSV(w *csv.Writer, s Sample) error {
	return w.Write([]string{
		s.Time.Format(time.RFC3339Nano), strconv.Itoa(s.Step), s.Phase, strconv.FormatInt(s.Bots, 10),
		strconv.Itoa(s.Users), strconv.FormatFloat(s.CPUPercent, 'f', 2, 64), strconv.FormatInt(s.RSSBytes, 10),
		strconv.FormatUint(s.HeapInuse, 10), strconv.FormatUint(s.Sys, 10), strconv.Itoa(s.Goroutines),
		strconv.FormatUint(uint64(s.NumGC), 10), strconv.FormatFloat(s.GCPause, 'f', 6, 64),
	})
}

func mib(b float64) string {
	if math.IsNaN(b) {
		return "–"
	}
	return fmt.Sprintf("%.1f", b/(1<<20))
}

func pct(v float64) string {
	if math.IsNaN(v) {
		return "–"
	}
	return fmt.Sprintf("%.1f", v)
}

func ms(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }

// RunInfo describes the run at the top of summary.md.
type RunInfo struct {
	Started time.Time
	Target  string
	Source  string
	Ramp    float64
	Steady  time.Duration
	Profile string
	Host    string
}

// writeSummary renders summary.md.
func writeSummary(w io.Writer, info RunInfo, samples []Sample, results []StepResult) {
	fmt.Fprintf(w, "# Load test %s\n\n", info.Started.Format("2006-01-02 15:04"))
	fmt.Fprintf(w, "- Server: `%s` (measured with %s)\n- Load generator host: %s\n", info.Target, info.Source, info.Host)
	fmt.Fprintf(w, "- Ramp: %.1f logins/s, steady phase: %s per step\n- Bot profile: %s\n\n", info.Ramp, info.Steady, info.Profile)
	fmt.Fprintln(w, "CPU is in percent of one core (200 = two cores busy). Memory in MiB: RSS from ps/docker, heap and Sys from the Go runtime.")
	fmt.Fprintln(w)

	idle := aggregate(samples, 0, "idle")
	fmt.Fprintln(w, "## Steady state")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Players | Online avg | CPU avg | CPU p95 | RSS avg | RSS max | Heap avg | Go Sys max | Goroutines | GC/min | Packets in/s | KiB/s out | KiB/s in |")
	fmt.Fprintln(w, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	if idle.N > 0 {
		fmt.Fprintf(w, "| 0 (idle) | 0 | %s | %s | %s | %s | %s | %s | %.0f | %.1f | – | – | – |\n", pct(idle.CPUAvg), pct(idle.CPUP95),
			mib(idle.RSSAvg), mib(idle.RSSMax), mib(idle.HeapAvg), mib(idle.SysMax), idle.GoroutinesAvg, idle.GCPerMin)
	}
	xs, cpus, rsss, heaps := []float64{0}, []float64{idle.CPUAvg}, []float64{idle.RSSAvg}, []float64{idle.HeapAvg}
	for _, r := range results {
		s := aggregate(samples, r.Bots, "steady")
		secs := r.SteadyPhase.Seconds()
		rate := func(v int64) string {
			if secs <= 0 {
				return "–"
			}
			return fmt.Sprintf("%.0f", float64(v)/secs)
		}
		kib := func(v int64) string {
			if secs <= 0 {
				return "–"
			}
			return fmt.Sprintf("%.1f", float64(v)/1024/secs)
		}
		// The bots' counters cover login + steady; the rates use the steady time
		// because the login burst is short.
		fmt.Fprintf(w, "| %d | %s | %s | %s | %s | %s | %s | %s | %.0f | %.1f | %s | %s | %s |\n", r.Bots, pct(s.BotsAvg), pct(s.CPUAvg), pct(s.CPUP95),
			mib(s.RSSAvg), mib(s.RSSMax), mib(s.HeapAvg), mib(s.SysMax), s.GoroutinesAvg, s.GCPerMin,
			rate(r.PacketsIn), kib(r.BytesIn), kib(r.BytesOut))
		xs = append(xs, s.BotsAvg)
		cpus = append(cpus, s.CPUAvg)
		rsss = append(rsss, s.RSSAvg)
		heaps = append(heaps, s.HeapAvg)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "\"Online avg\" counts the bots still in the world (the server may disconnect some); the fit below uses it.")
	fmt.Fprintln(w, "\"KiB/s out\" is what the server sent to the bots, \"KiB/s in\" what the bots sent.")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Login phase")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Players | OK | Failed | Login p50 ms | Login p95 ms | Login max ms | Phase s | CPU max | RSS max | Go Sys max |")
	fmt.Fprintln(w, "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, r := range results {
		s := aggregate(samples, r.Bots, "login")
		fmt.Fprintf(w, "| %d | %d | %d | %s | %s | %s | %.1f | %s | %s | %s |\n", r.Bots, r.LoginOK, r.LoginFail,
			ms(durPercentile(r.LoginTimes, 50)), ms(durPercentile(r.LoginTimes, 95)), ms(durPercentile(r.LoginTimes, 100)),
			r.LoginPhase.Seconds(), pct(s.CPUMax), mib(s.RSSMax), mib(s.SysMax))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Bot activity")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Players | Walks | Attacks | Kills | Deaths | Disconnects | Errors |")
	fmt.Fprintln(w, "|---:|---:|---:|---:|---:|---:|---|")
	for _, r := range results {
		fmt.Fprintf(w, "| %d | %d | %d | %d | %d | %d | %s |\n", r.Bots, r.Walks, r.Attacks, r.Kills, r.Deaths, r.Disconnects, errorList(r.Errors))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Cost per player (linear fit of the steady state over the online players)")
	fmt.Fprintln(w)
	fit := func(label string, y []float64, unit func(float64) string) {
		if a, b, ok := linearFit(xs, y); ok {
			fmt.Fprintf(w, "- %s: %s per player + %s base\n", label, unit(a), unit(b))
		} else {
			fmt.Fprintf(w, "- %s: not enough data\n", label)
		}
	}
	fit("CPU", cpus, func(v float64) string { return fmt.Sprintf("%.2f%% of a core", v) })
	fit("RSS", rsss, func(v float64) string { return mib(v) + " MiB" })
	fit("Go heap", heaps, func(v float64) string { return mib(v) + " MiB" })
}

func errorList(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%d× %s", m[k], strings.ReplaceAll(k, "|", "/")))
	}
	return strings.Join(out, "; ")
}
