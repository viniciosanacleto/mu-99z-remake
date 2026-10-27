package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// serverSample is the /debug/load body of muserver -metrics-addr.
type serverSample struct {
	UnixMilli    int64   `json:"unix_ms"`
	CPUUserSec   float64 `json:"cpu_user_s"`
	CPUSysSec    float64 `json:"cpu_sys_s"`
	MaxRSSBytes  int64   `json:"max_rss_bytes"`
	Goroutines   int     `json:"goroutines"`
	HeapAlloc    uint64  `json:"heap_alloc"`
	HeapInuse    uint64  `json:"heap_inuse"`
	StackInuse   uint64  `json:"stack_inuse"`
	Sys          uint64  `json:"sys"`
	NumGC        uint32  `json:"num_gc"`
	GCPauseTotal float64 `json:"gc_pause_total_s"`
	Users        []struct {
		Code  int `json:"code"`
		Users int `json:"users"`
	} `json:"users"`
}

// Sample is one row of samples.csv.
type Sample struct {
	Time       time.Time
	Step       int    // bots wanted in this step (0 = idle baseline)
	Phase      string // idle, login, steady, cooldown
	Bots       int64  // bots in the world
	CPUPercent float64
	RSSBytes   int64 // -1 when unknown
	HeapInuse  uint64
	Sys        uint64
	Goroutines int
	NumGC      uint32
	GCPause    float64 // seconds of GC pause since the previous sample
	Users      int     // users the GameServers report
}

// probe reads the server once. Any source may be missing.
type probe struct {
	metricsURL string
	pid        int
	container  string
	client     *http.Client
	prev       *serverSample
}

type probeResult struct {
	server     *serverSample
	cpuPercent float64 // -1 when unknown
	rss        int64   // -1 when unknown
}

func (p *probe) read(ctx context.Context) (probeResult, error) {
	r := probeResult{cpuPercent: -1, rss: -1}
	var errs []string
	if p.metricsURL != "" {
		s, err := p.fetch(ctx)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			r.server = s
			if p.prev != nil {
				r.cpuPercent = cpuPercent(p.prev, s)
			}
			p.prev = s
		}
	}
	switch {
	case p.container != "":
		out, err := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{.CPUPerc}};{{.MemUsage}}", p.container).Output()
		if err != nil {
			errs = append(errs, "docker stats: "+err.Error())
			break
		}
		cpu, rss, err := parseDockerStats(string(out))
		if err != nil {
			errs = append(errs, err.Error())
			break
		}
		r.rss = rss
		if r.cpuPercent < 0 {
			r.cpuPercent = cpu
		}
	case p.pid > 0:
		out, err := exec.CommandContext(ctx, "ps", "-o", "rss=,%cpu=", "-p", strconv.Itoa(p.pid)).Output()
		if err != nil {
			errs = append(errs, "ps: "+err.Error())
			break
		}
		rss, cpu, err := parsePS(string(out))
		if err != nil {
			errs = append(errs, err.Error())
			break
		}
		r.rss = rss
		if r.cpuPercent < 0 {
			r.cpuPercent = cpu
		}
	}
	if len(errs) > 0 {
		return r, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return r, nil
}

func (p *probe) fetch(ctx context.Context) (*serverSample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.metricsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", p.metricsURL, resp.Status)
	}
	var s serverSample
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// cpuPercent is the process CPU time between two samples over the wall
// time, in percent of one core (200 = two cores busy).
func cpuPercent(a, b *serverSample) float64 {
	wall := float64(b.UnixMilli-a.UnixMilli) / 1000
	if wall <= 0 {
		return 0
	}
	used := (b.CPUUserSec + b.CPUSysSec) - (a.CPUUserSec + a.CPUSysSec)
	return max(used, 0) / wall * 100
}

// parsePS reads "ps -o rss=,%cpu=" (RSS in KiB).
func parsePS(out string) (rss int64, cpu float64, err error) {
	f := strings.Fields(out)
	if len(f) < 2 {
		return 0, 0, fmt.Errorf("ps: unexpected output %q", out)
	}
	kb, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("ps rss %q: %w", f[0], err)
	}
	cpu, err = strconv.ParseFloat(strings.Replace(f[1], ",", ".", 1), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("ps cpu %q: %w", f[1], err)
	}
	return kb * 1024, cpu, nil
}

// parseDockerStats reads "{{.CPUPerc}};{{.MemUsage}}", e.g.
// "12.34%;150.2MiB / 7.66GiB".
func parseDockerStats(out string) (cpu float64, rss int64, err error) {
	parts := strings.SplitN(strings.TrimSpace(out), ";", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("docker stats: unexpected output %q", out)
	}
	cpu, err = strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%"), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("docker cpu %q: %w", parts[0], err)
	}
	used, _, _ := strings.Cut(parts[1], "/")
	rss, err = parseSize(strings.TrimSpace(used))
	return cpu, rss, err
}

// parseSize reads the docker sizes ("512B", "1.5KiB", "150.2MiB", "1GB").
func parseSize(s string) (int64, error) {
	units := []struct {
		suffix string
		mult   float64
	}{
		{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
		{"kB", 1e3}, {"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1},
	}
	for _, u := range units {
		if v, ok := strings.CutSuffix(s, u.suffix); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return 0, fmt.Errorf("size %q: %w", s, err)
			}
			return int64(f * u.mult), nil
		}
	}
	return 0, fmt.Errorf("size %q: unknown unit", s)
}
