package worker

import (
	"testing"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var eventsEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func makeEvents(lineNumbers ...int32) []db.Event {
	events := make([]db.Event, len(lineNumbers))
	for i, lineNumber := range lineNumbers {
		events[i] = db.Event{
			LineNumber: lineNumber,
			Time:       eventsEpoch.Add(time.Duration(i) * time.Second),
		}
	}

	return events
}

func eventLineNumbers(events []db.Event) []int32 {
	lineNumbers := make([]int32, len(events))
	for i, event := range events {
		lineNumbers[i] = event.LineNumber
	}

	return lineNumbers
}

func TestFilterEventsEmpty(t *testing.T) {
	assert.Empty(t, FilterEvents(nil, func(db.Event) bool { return true }))
	assert.Empty(t, FilterEvents([]db.Event{}, func(db.Event) bool { return true }))
}

func TestFilterEventsAllAndNone(t *testing.T) {
	events := makeEvents(1, 2, 3)

	assert.Equal(t, []int32{1, 2, 3}, eventLineNumbers(FilterEvents(events, func(db.Event) bool { return true })))
	assert.Empty(t, FilterEvents(events, func(db.Event) bool { return false }))
}

func TestFilterEventsPreservesOrder(t *testing.T) {
	events := makeEvents(5, 1, 4, 2, 3)

	filtered := FilterEvents(events, func(e db.Event) bool { return e.LineNumber%2 == 1 })

	assert.Equal(t, []int32{5, 1, 3}, eventLineNumbers(filtered))
}

func TestFilterEventsDoesNotMutateInput(t *testing.T) {
	events := makeEvents(1, 2, 3)

	filtered := FilterEvents(events, func(e db.Event) bool { return e.LineNumber != 2 })
	require.Len(t, filtered, 2)

	filtered[0].LineNumber = 99

	assert.Equal(t, []int32{1, 2, 3}, eventLineNumbers(events))
}

func TestFilterEventsPredicateSeesEachEvent(t *testing.T) {
	events := makeEvents(1, 2, 3)
	seen := []int32{}

	FilterEvents(events, func(e db.Event) bool {
		seen = append(seen, e.LineNumber)

		return true
	})

	assert.Equal(t, []int32{1, 2, 3}, seen)
}

func TestSplitByEmpty(t *testing.T) {
	assert.Empty(t, SplitBy[db.Event, string](nil, db.Event.UserKey))
	assert.Empty(t, SplitBy([]db.Event{}, db.Event.UserKey))
}

func TestSplitByGroupsAndPreservesOrder(t *testing.T) {
	events := []db.Event{
		{LineNumber: 1, User: "alice"},
		{LineNumber: 2, User: "bob"},
		{LineNumber: 3, User: "alice"},
		{LineNumber: 4, User: ""},
		{LineNumber: 5, User: "bob"},
	}

	groups := SplitBy(events, db.Event.UserKey)

	require.Len(t, groups, 3)
	assert.Equal(t, []int32{1, 3}, eventLineNumbers(groups["alice"]))
	assert.Equal(t, []int32{2, 5}, eventLineNumbers(groups["bob"]))
	assert.Equal(t, []int32{4}, eventLineNumbers(groups[""]), "empty key is grouped, not dropped")
}

func TestSplitByNonStringKey(t *testing.T) {
	events := makeEvents(1, 2, 1, 3)

	groups := SplitBy(events, func(e db.Event) int32 { return e.LineNumber })

	require.Len(t, groups, 3)
	assert.Equal(t, []int32{1, 1}, eventLineNumbers(groups[1]))
	assert.Equal(t, []int32{2}, eventLineNumbers(groups[2]))
	assert.Equal(t, []int32{3}, eventLineNumbers(groups[3]))
}

func TestSplitByDoesNotMutateInput(t *testing.T) {
	events := []db.Event{{LineNumber: 1, User: "alice"}}

	groups := SplitBy(events, db.Event.UserKey)
	require.Len(t, groups["alice"], 1)

	groups["alice"][0].LineNumber = 99

	assert.Equal(t, int32(1), events[0].LineNumber)
}

func TestCountUniqueEmpty(t *testing.T) {
	assert.Zero(t, CountUnique[db.Event, string](nil, db.Event.UserKey))
	assert.Zero(t, CountUnique([]db.Event{}, db.Event.UserKey))
}

func TestCountUniqueCountsDistinct(t *testing.T) {
	events := []db.Event{
		{User: "alice"},
		{User: "bob"},
		{User: "alice"},
		{User: ""},
	}

	assert.Equal(t, 3, CountUnique(events, db.Event.UserKey), "empty key counts as a value")
	assert.Equal(t, 1, CountUnique(events, db.Event.ClientIPKey), "identical keys count once")
}

func TestCountUniqueNonStringKey(t *testing.T) {
	events := makeEvents(1, 2, 1, 3)

	assert.Equal(t, 3, CountUnique(events, func(e db.Event) int32 { return e.LineNumber }))
	assert.Equal(t, 1, CountUnique(events, func(db.Event) bool { return true }))
}

func TestCountUniqueDoesNotMutateInput(t *testing.T) {
	events := []db.Event{{LineNumber: 1, User: "alice"}}

	CountUnique(events, func(e db.Event) int32 { return e.LineNumber })

	assert.Equal(t, int32(1), events[0].LineNumber)
}

func TestEventRateEmptyAndSingle(t *testing.T) {
	assert.Zero(t, EventRate(nil))
	assert.Zero(t, EventRate([]db.Event{}))
	assert.Zero(t, EventRate(makeEvents(1)))
}

func TestEventRateZeroSpan(t *testing.T) {
	at := eventsEpoch
	events := []db.Event{{Time: at}, {Time: at}, {Time: at}}

	assert.Zero(t, EventRate(events))
}

func TestEventRateCountsBothEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		count  int
		step   time.Duration
		expect float64
	}{
		{name: "two events one second apart", count: 2, step: time.Second, expect: 2},
		{name: "four events over ten seconds", count: 4, step: 10 * time.Second / 3, expect: 0.4},
		{name: "eleven events one second apart", count: 11, step: time.Second, expect: 1.1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := make([]db.Event, test.count)
			for i := range test.count {
				events[i] = db.Event{Time: eventsEpoch.Add(time.Duration(i) * test.step)}
			}

			assert.InDelta(t, test.expect, EventRate(events), 1e-9)

			expectedSpan := time.Duration(test.count-1) * test.step
			assert.InDelta(t, float64(test.count)/expectedSpan.Seconds(), EventRate(events), 1e-9)
		})
	}
}

func TestEventRateUsesFirstAndLastEvent(t *testing.T) {
	events := []db.Event{
		{Time: eventsEpoch.Add(0)},
		{Time: eventsEpoch.Add(45 * time.Second)},
		{Time: eventsEpoch.Add(60 * time.Second)},
	}

	assert.InDelta(t, 3.0/60.0, EventRate(events), 1e-9, "middle events do not affect the span")
}

func TestEventRateDescendingSpan(t *testing.T) {
	events := []db.Event{
		{Time: eventsEpoch.Add(time.Minute)},
		{Time: eventsEpoch.Add(0)},
	}

	assert.Zero(t, EventRate(events), "non-positive span returns 0")
}

func TestFailureBucketsEmpty(t *testing.T) {
	assert.Empty(t, FailureBuckets(nil))
	assert.Empty(t, FailureBuckets([]db.Event{}))
}

func TestFailureBucketsNoFailures(t *testing.T) {
	events := []db.Event{
		{Action: ActionLoginSuccess},
		{Action: "Logout"},
		{Action: ActionLoginSuccess},
	}

	assert.Empty(t, FailureBuckets(events))
}

func TestFailureBucketsFailuresOnly(t *testing.T) {
	events := makeEvents(1, 2, 3)
	for i := range events {
		events[i].Action = ActionLoginFail
	}

	buckets := FailureBuckets(events)

	require.Len(t, buckets, 1)
	assert.Equal(t, []int32{1, 2, 3}, eventLineNumbers(buckets[0]), "no successes yields a single bucket")
}

func TestFailureBucketsSplitsOnSuccess(t *testing.T) {
	events := []db.Event{
		{LineNumber: 1, Action: ActionLoginFail},
		{LineNumber: 2, Action: ActionLoginFail},
		{LineNumber: 3, Action: ActionLoginSuccess},
		{LineNumber: 4, Action: ActionLoginFail},
		{LineNumber: 5, Action: ActionLoginSuccess},
		{LineNumber: 6, Action: ActionLoginFail},
	}

	buckets := FailureBuckets(events)

	require.Len(t, buckets, 3)
	assert.Equal(t, []int32{1, 2}, eventLineNumbers(buckets[0]))
	assert.Equal(t, []int32{4}, eventLineNumbers(buckets[1]))
	assert.Equal(t, []int32{6}, eventLineNumbers(buckets[2]), "trailing failures form a final bucket")
}

func TestFailureBucketsSkipsEmptyBuckets(t *testing.T) {
	events := []db.Event{
		{Action: ActionLoginSuccess},
		{LineNumber: 1, Action: ActionLoginFail},
		{Action: ActionLoginSuccess},
		{Action: ActionLoginSuccess},
		{LineNumber: 2, Action: ActionLoginSuccess},
	}

	buckets := FailureBuckets(events)

	require.Len(t, buckets, 1)
	assert.Equal(t, []int32{1}, eventLineNumbers(buckets[0]))
}

func TestFailureBucketsIgnoresOtherActions(t *testing.T) {
	events := []db.Event{
		{LineNumber: 1, Action: ActionLoginFail},
		{LineNumber: 2, Action: "Logout"},
		{LineNumber: 3, Action: ActionLoginFail},
		{LineNumber: 4, Action: "Logout"},
	}

	buckets := FailureBuckets(events)

	require.Len(t, buckets, 1)
	assert.Equal(t, []int32{1, 3}, eventLineNumbers(buckets[0]), "other actions neither add to nor close a bucket")
}

func TestFailureBucketsDoesNotMutateInput(t *testing.T) {
	events := []db.Event{{LineNumber: 1, Action: ActionLoginFail}}

	buckets := FailureBuckets(events)
	require.Len(t, buckets, 1)
	require.Len(t, buckets[0], 1)

	buckets[0][0].LineNumber = 99

	assert.Equal(t, int32(1), events[0].LineNumber)
}

func TestLineNumbersEmpty(t *testing.T) {
	assert.Empty(t, LineNumbers(nil))
	assert.Empty(t, LineNumbers([]db.Event{}))
}

func TestLineNumbersPreservesOrder(t *testing.T) {
	assert.Equal(t, []int32{9, 3, 7}, LineNumbers(makeEvents(9, 3, 7)))
	assert.Equal(t, []int32{0, 0}, LineNumbers(makeEvents(0, 0)), "duplicates are preserved")
}

func TestLineNumbersDoesNotMutateInput(t *testing.T) {
	events := makeEvents(1, 2)

	lineNumbers := LineNumbers(events)
	require.Len(t, lineNumbers, 2)

	lineNumbers[0] = 99

	assert.Equal(t, []int32{1, 2}, eventLineNumbers(events))
}
