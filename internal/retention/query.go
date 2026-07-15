// Package retention provides whitelisted, batched data-retention operations.
package retention

import "time"

type querySpec struct {
	countSQL       string
	deleteSQL      string
	requireExpired bool
	windowCount    int
}

func (q querySpec) rangeArgs(start, end, now time.Time) []any {
	windows := q.windowCount
	if windows == 0 {
		windows = 1
	}
	args := make([]any, 0, windows*3)
	for range windows {
		args = append(args, start, end)
		if q.requireExpired {
			args = append(args, now)
		}
	}
	return args
}

type datasetPlan struct {
	dataset Dataset
	queries []querySpec
}
