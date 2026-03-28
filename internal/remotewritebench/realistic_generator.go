package remotewritebench

import (
	"fmt"
	"sort"
	"sync"

	"github.com/prometheus/prometheus/prompb"
)

const (
	realisticMetricName = "bench_container_cpu_usage_seconds_total"
	stableRatio         = 0.9
)

var (
	realisticJobs = []string{
		"kubelet",
		"kube-state-metrics",
		"node-exporter",
	}
	realisticNamespaces = []string{
		"default",
		"kube-system",
		"monitoring",
		"prod",
		"staging",
	}
	realisticContainers = []string{
		"app",
		"sidecar",
		"init",
	}
)

type churnMeta struct {
	jobIdx       int
	namespaceIdx int
	containerIdx int
	nodeIdx      int
	podIndex     int
	generation   int
}

type realisticPool struct {
	stable    []*seriesState
	churn     []*seriesState
	churnMeta []churnMeta
	mu        sync.Mutex
}

func buildRealisticSeriesStates(cfg Config) (*realisticPool, error) {
	if cfg.Series <= 0 {
		return nil, fmt.Errorf("series must be greater than 0")
	}

	nodeCount := max(1, cfg.Series/100)

	stableCount := int(float64(cfg.Series) * stableRatio)
	churnCount := cfg.Series - stableCount

	stable := make([]*seriesState, 0, stableCount)
	for i := 0; i < stableCount; i++ {
		jobIdx := i % len(realisticJobs)
		nsIdx := i % len(realisticNamespaces)
		cIdx := i % len(realisticContainers)
		nodeIdx := i % nodeCount
		stable = append(stable, newRealisticSeriesState(
			realisticJobs[jobIdx],
			realisticNamespaces[nsIdx],
			fmt.Sprintf("pod-%s-%d-0", realisticNamespaces[nsIdx], i),
			realisticContainers[cIdx],
			fmt.Sprintf("node-%d", nodeIdx),
		))
	}

	churnSlots := make([]*seriesState, 0, churnCount)
	churnMetas := make([]churnMeta, 0, churnCount)
	for i := 0; i < churnCount; i++ {
		meta := churnMeta{
			jobIdx:       i % len(realisticJobs),
			namespaceIdx: i % len(realisticNamespaces),
			containerIdx: i % len(realisticContainers),
			nodeIdx:      i % nodeCount,
			podIndex:     i,
			generation:   0,
		}
		churnSlots = append(churnSlots, newChurnSeriesState(&meta))
		churnMetas = append(churnMetas, meta)
	}

	return &realisticPool{
		stable:    stable,
		churn:     churnSlots,
		churnMeta: churnMetas,
	}, nil
}

// allStates returns a combined slice of stable + current churn series.
// Caller must not hold the lock.
func (p *realisticPool) allStates() []*seriesState {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*seriesState, 0, len(p.stable)+len(p.churn))
	out = append(out, p.stable...)
	out = append(out, p.churn...)
	return out
}

// maybeChurn rotates one churn slot per sequence call, cycling through all
// churn slots. Each slot's pod generation is incremented when it is rotated,
// producing a new time series that simulates a pod restart.
func (p *realisticPool) maybeChurn(sequence int64) {
	if len(p.churn) == 0 {
		return
	}

	slotIndex := int(sequence % int64(len(p.churn)))

	p.mu.Lock()
	defer p.mu.Unlock()

	p.churnMeta[slotIndex].generation++
	p.churn[slotIndex] = newChurnSeriesState(&p.churnMeta[slotIndex])
}

func newChurnSeriesState(meta *churnMeta) *seriesState {
	return newRealisticSeriesState(
		realisticJobs[meta.jobIdx],
		realisticNamespaces[meta.namespaceIdx],
		fmt.Sprintf("pod-%s-%d-%d", realisticNamespaces[meta.namespaceIdx], meta.podIndex, meta.generation),
		realisticContainers[meta.containerIdx],
		fmt.Sprintf("node-%d", meta.nodeIdx),
	)
}

func newRealisticSeriesState(job, namespace, pod, container, instance string) *seriesState {
	labels := []prompb.Label{
		{Name: "__name__", Value: realisticMetricName},
		{Name: "container", Value: container},
		{Name: "instance", Value: instance},
		{Name: "job", Value: job},
		{Name: "namespace", Value: namespace},
		{Name: "pod", Value: pod},
	}
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].Name < labels[j].Name
	})
	return &seriesState{labels: labels}
}
