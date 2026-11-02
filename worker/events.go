package worker

import (
	"github.com/Henelik/log-analyzer/db"
)

// FilterEvents returns the events for which keep returns true, preserving order.  It does not mutate events.
func FilterEvents(events []db.Event, keep func(db.Event) bool) []db.Event {
	filtered := make([]db.Event, 0, len(events))
	for _, event := range events {
		if keep(event) {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// SplitBy groups items by the key returned by key, preserving order within each group.  It does not mutate items.
func SplitBy[T any, K comparable](items []T, key func(T) K) map[K][]T {
	groups := make(map[K][]T)

	for _, item := range items {
		k := key(item)
		groups[k] = append(groups[k], item)
	}

	return groups
}

// EventRate returns the average number of events per second across the span of events,
// counting both endpoints.  Events are assumed to be time-sorted.  Returns 0 for fewer than
// two events or a non-positive span.
func EventRate(events []db.Event) float64 {
	if len(events) < 2 {
		return 0
	}

	span := events[len(events)-1].Time.Sub(events[0].Time)
	if span <= 0 {
		return 0
	}

	return float64(len(events)) / span.Seconds()
}

// FailureBuckets groups the login-fail events in events into runs of consecutive failures.
// Each login success closes the current bucket, so a bucket holds exactly the failures that
// preceded it.  Failures after the last success form a final bucket, so a user with no
// successes yields a single bucket holding every failure.
// Events are assumed to be time-sorted and to belong to a single user.
func FailureBuckets(events []db.Event) [][]db.Event {
	buckets := make([][]db.Event, 0)
	current := []db.Event{}

	for _, event := range events {
		switch event.Action {
		case ActionLoginFail:
			current = append(current, event)
		case ActionLoginSuccess:
			if len(current) > 0 {
				buckets = append(buckets, current)
				current = []db.Event{}
			}
		}
	}

	if len(current) > 0 {
		buckets = append(buckets, current)
	}

	return buckets
}

// LineNumbers returns the line numbers of events, in order.  It does not mutate events.
func LineNumbers(events []db.Event) []int32 {
	lineNumbers := make([]int32, len(events))
	for i, event := range events {
		lineNumbers[i] = event.LineNumber
	}

	return lineNumbers
}
