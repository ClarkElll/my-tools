package remotewritebench

import (
	"sort"
	"strings"
	"testing"
	"time"
)

func realisticCfg(series int) Config {
	cfg, _ := Config{
		URL:             "http://localhost:9090/api/v1/write",
		Series:          series,
		RequestInterval: time.Second,
		SampleInterval:  15 * time.Second,
		Concurrency:     1,
		Timeout:         time.Second,
		Realistic:       true,
	}.Normalized()
	return cfg
}

func TestBuildRealisticSeriesStatesCount(t *testing.T) {
	cfg := realisticCfg(100)
	pool, err := buildRealisticSeriesStates(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	states := pool.allStates()
	if len(states) != 100 {
		t.Fatalf("expected 100 series, got %d", len(states))
	}
}

func TestBuildRealisticSeriesStatesLabels(t *testing.T) {
	cfg := realisticCfg(50)
	pool, err := buildRealisticSeriesStates(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	requiredLabels := []string{"__name__", "job", "namespace", "pod", "container", "instance"}

	for i, s := range pool.allStates() {
		labelMap := make(map[string]string, len(s.labels))
		for _, l := range s.labels {
			labelMap[l.Name] = l.Value
		}
		for _, name := range requiredLabels {
			if _, ok := labelMap[name]; !ok {
				t.Fatalf("series %d missing label %q", i, name)
			}
		}
	}
}

func TestBuildRealisticSeriesStatesLabelsSorted(t *testing.T) {
	cfg := realisticCfg(20)
	pool, err := buildRealisticSeriesStates(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, s := range pool.allStates() {
		if !sort.SliceIsSorted(s.labels, func(a, b int) bool {
			return s.labels[a].Name < s.labels[b].Name
		}) {
			t.Fatalf("series %d labels not sorted", i)
		}
	}
}

func TestStableSeriesDoNotChange(t *testing.T) {
	cfg := realisticCfg(100)
	pool, err := buildRealisticSeriesStates(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// snapshot stable labels before churn
	stableCount := len(pool.stable)
	before := make([]string, stableCount)
	for i, s := range pool.stable {
		before[i] = labelsKey(s)
	}

	// run many churn cycles
	for seq := int64(0); seq < 1000; seq++ {
		pool.maybeChurn(seq)
	}

	for i, s := range pool.stable {
		if labelsKey(s) != before[i] {
			t.Fatalf("stable series %d changed after churn", i)
		}
	}
}

func TestChurnSeriesGenerationIncreases(t *testing.T) {
	cfg := realisticCfg(100)
	pool, err := buildRealisticSeriesStates(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pool.churn) == 0 {
		t.Skip("no churn slots")
	}

	// record initial pod label for slot 0
	initialPod := podLabel(pool.churn[0])

	// advance enough sequences to rotate slot 0 at least once
	cycles := int64(len(pool.churn)) + 1
	for seq := int64(0); seq < cycles; seq++ {
		pool.maybeChurn(seq)
	}

	newPod := podLabel(pool.churn[0])
	if newPod == initialPod {
		t.Fatalf("expected pod label to change after churn, still %q", newPod)
	}
}

func TestAllStatesLength(t *testing.T) {
	for _, n := range []int{1, 10, 99, 100, 101, 1000} {
		cfg := realisticCfg(n)
		pool, err := buildRealisticSeriesStates(cfg)
		if err != nil {
			t.Fatalf("series=%d: unexpected error: %v", n, err)
		}
		if got := len(pool.allStates()); got != n {
			t.Fatalf("series=%d: expected %d states, got %d", n, n, got)
		}
	}
}

func labelsKey(s *seriesState) string {
	parts := make([]string, len(s.labels))
	for i, l := range s.labels {
		parts[i] = l.Name + "=" + l.Value
	}
	return strings.Join(parts, ",")
}

func podLabel(s *seriesState) string {
	for _, l := range s.labels {
		if l.Name == "pod" {
			return l.Value
		}
	}
	return ""
}
