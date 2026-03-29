package remotewritebench

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/prometheus/prometheus/prompb"
)

type seriesState struct {
	labels []prompb.Label
}

func buildSeriesStates(cfg Config) ([]*seriesState, error) {
	states := make([]*seriesState, 0, cfg.Series)
	labelWidth := len(strconv.Itoa(cfg.Series - 1))
	if labelWidth < 1 {
		labelWidth = 1
	}

	for seriesIndex := 0; seriesIndex < cfg.Series; seriesIndex++ {
		labels := make([]prompb.Label, 0, 3)
		labels = append(labels, prompb.Label{Name: "__name__", Value: DefaultMetricName})
		labels = append(labels, prompb.Label{
			Name:  "series_id",
			Value: fmt.Sprintf("series_id-%0*d", labelWidth, seriesIndex),
		})
		if cfg.UTF8Label {
			labels = append(labels, prompb.Label{Name: UTF8LabelName, Value: UTF8LabelValue})
		}

		sort.Slice(labels, func(i, j int) bool {
			return labels[i].Name < labels[j].Name
		})

		states = append(states, &seriesState{
			labels: labels,
		})
	}

	return states, nil
}
