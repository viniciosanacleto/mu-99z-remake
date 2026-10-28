package main

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseSteps(t *testing.T) {
	got, err := parseSteps(" 1, 10,50 ,100,")
	if err != nil || len(got) != 4 || got[0] != 1 || got[3] != 100 {
		t.Fatalf("= %v %v", got, err)
	}
	for _, bad := range []string{"", "0", "1,x", "-3"} {
		if _, err := parseSteps(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseFlags(t *testing.T) {
	if _, err := parseFlags(nil); err == nil {
		t.Error("no measurement source accepted")
	}
	if _, err := parseFlags([]string{"-pid", "1", "-docker", "x"}); err == nil {
		t.Error("-pid with -docker accepted")
	}
	c, err := parseFlags([]string{"-metrics", "http://x/debug/load", "-steps", "2,4", "-ramp", "10", "-chat-every", "0"})
	if err != nil {
		t.Fatal(err)
	}
	if c.gameHost != "127.0.0.1" {
		t.Errorf("game host %q, want the -connect host", c.gameHost)
	}
	if c, _ := parseFlags([]string{"-pid", "1", "-game-host", "server"}); c.gameHost != "" {
		t.Errorf("-game-host server = %q", c.gameHost)
	}
	if len(c.steps) != 2 || c.ramp != 10 || c.profile.ChatEvery != 0 || !strings.HasPrefix(c.out, "loadtest-") {
		t.Fatalf("= %+v", c)
	}
}

func TestParsePS(t *testing.T) {
	rss, cpu, err := parsePS("  51234   12.5\n")
	if err != nil || rss != 51234*1024 || cpu != 12.5 {
		t.Fatalf("= %d %v %v", rss, cpu, err)
	}
	if _, cpu, _ := parsePS("100 3,5"); cpu != 3.5 { // decimal comma locales
		t.Errorf("comma cpu = %v", cpu)
	}
	if _, _, err := parsePS(""); err == nil {
		t.Error("empty accepted")
	}
}

func TestParseDockerStats(t *testing.T) {
	cpu, rss, err := parseDockerStats("12.34%;150.5MiB / 7.66GiB\n")
	if err != nil || cpu != 12.34 || rss != int64(150.5*(1<<20)) {
		t.Fatalf("= %v %d %v", cpu, rss, err)
	}
	if _, rss, _ := parseDockerStats("0.00%;1.5GiB / 2GiB"); rss != 3<<29 {
		t.Errorf("GiB = %d", rss)
	}
	if _, rss, _ := parseDockerStats("1%;900kB / 1GB"); rss != 900000 {
		t.Errorf("kB = %d", rss)
	}
	for _, bad := range []string{"", "x%;1MiB / 2MiB", "1%;1 parsecs / 2MiB"} {
		if _, _, err := parseDockerStats(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCPUPercent(t *testing.T) {
	a := &serverSample{UnixMilli: 1000, CPUUserSec: 1, CPUSysSec: 0.5}
	b := &serverSample{UnixMilli: 3000, CPUUserSec: 2, CPUSysSec: 1.5}
	if got := cpuPercent(a, b); got != 100 { // 2 s of CPU in 2 s
		t.Fatalf("= %v", got)
	}
	if got := cpuPercent(b, b); got != 0 {
		t.Fatalf("same sample = %v", got)
	}
}

func TestStatsHelpers(t *testing.T) {
	v := []float64{5, 1, 4, 2, 3}
	if mean(v) != 3 || maxOf(v) != 5 || percentile(v, 50) != 3 || percentile(v, 95) != 5 || percentile(v, 0) != 1 {
		t.Fatalf("mean %v max %v p50 %v p95 %v", mean(v), maxOf(v), percentile(v, 50), percentile(v, 95))
	}
	if !math.IsNaN(mean(nil)) || !math.IsNaN(percentile(nil, 50)) {
		t.Error("empty input")
	}
	if got := durPercentile([]time.Duration{3 * time.Second, time.Second, 2 * time.Second}, 50); got != 2*time.Second {
		t.Errorf("duration p50 = %v", got)
	}
	slope, icpt, ok := linearFit([]float64{0, 10, 50, 100}, []float64{1, 3, 11, 21})
	if !ok || math.Abs(slope-0.2) > 1e-9 || math.Abs(icpt-1) > 1e-9 {
		t.Fatalf("fit = %v %v %v", slope, icpt, ok)
	}
	if _, _, ok := linearFit([]float64{0, 1}, []float64{math.NaN(), 2}); ok {
		t.Error("fit with one point")
	}
}

func TestAggregate(t *testing.T) {
	t0 := time.Unix(0, 0)
	samples := []Sample{
		{Time: t0, Step: 10, Phase: "steady", Bots: 9, CPUPercent: 10, RSSBytes: 100, HeapInuse: 50, Sys: 200, Goroutines: 30, NumGC: 4, GCPause: 0.001},
		{Time: t0.Add(30 * time.Second), Step: 10, Phase: "steady", Bots: 9, CPUPercent: 20, RSSBytes: 300, HeapInuse: 70, Sys: 260, Goroutines: 40, NumGC: 6, GCPause: 0.002},
		{Time: t0.Add(time.Minute), Step: 10, Phase: "steady", Bots: 6, CPUPercent: -1, RSSBytes: -1, NumGC: 0},
		{Time: t0, Step: 10, Phase: "login", CPUPercent: 99},
	}
	s := aggregate(samples, 10, "steady")
	if s.N != 3 || s.BotsAvg != 8 || s.CPUAvg != 15 || s.CPUMax != 20 || s.RSSAvg != 200 || s.RSSMax != 300 || s.HeapAvg != 60 ||
		s.SysMax != 260 || s.GoroutinesAvg != 35 || math.Abs(s.GCPause-0.003) > 1e-9 {
		t.Fatalf("= %+v", s)
	}
}

func TestErrorKey(t *testing.T) {
	err := errors.New("read tcp 127.0.0.1:50123->127.0.0.1:55900: connection reset by peer")
	if got := errorKey(err); got != "read tcp <addr>-><addr>: connection reset by peer" {
		t.Fatalf("= %q", got)
	}
}

func TestWriteSummary(t *testing.T) {
	t0 := time.Unix(0, 0)
	var samples []Sample
	for i, step := range []int{0, 1, 10} {
		phase := "steady"
		if step == 0 {
			phase = "idle"
		}
		for k := 0; k < 3; k++ {
			samples = append(samples, Sample{Time: t0.Add(time.Duration(i*10+k) * time.Second), Step: step, Phase: phase,
				Bots: int64(step), CPUPercent: float64(1 + step), RSSBytes: int64(10+step) << 20, HeapInuse: uint64(5+step) << 20, Sys: 20 << 20, Goroutines: 10 + step})
		}
	}
	results := []StepResult{
		{Bots: 1, LoginOK: 1, LoginTimes: []time.Duration{40 * time.Millisecond}, SteadyPhase: 10 * time.Second, PacketsIn: 100, BytesIn: 10240},
		{Bots: 10, LoginOK: 9, LoginFail: 1, Errors: map[string]int{"login refused": 1}, SteadyPhase: 10 * time.Second},
	}
	var sb strings.Builder
	writeSummary(&sb, RunInfo{Started: t0, Target: "x:1", Source: "ps", Ramp: 5, Steady: 10 * time.Second}, samples, results)
	out := sb.String()
	for _, want := range []string{
		"| 0 (idle) | 0 | 1.0 |",
		"| 1 | 1.0 | 2.0 | 2.0 | 11.0 | 11.0 | 6.0 | 20.0 | 11 | 0.0 | 10 | 1.0 | 0.0 |",
		"| 10 | 9 | 1 |",
		"1× login refused",
		"- CPU: 1.00% of a core per player + 1.00% of a core base",
		"- RSS: 1.0 MiB per player + 10.0 MiB base",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary misses %q:\n%s", want, out)
		}
	}
}
