package worker

import (
	"math"
	"slices"

	"github.com/Henelik/log-analyzer/db"
)

const (
	bruteForceThresh = 10
	bruteForceRate   = 1.0 / 60 // the number of brute force events per second to be considered a detection; one per minute

	ActionLoginFail    = "Login Fail"
	ActionLoginSuccess = "Login Success"
)

type Detector func([]db.Event) []db.Detection

var Detectors = []Detector{
	BruteForceDetector,
}

var LoginEvents = []string{ActionLoginFail, ActionLoginSuccess}

// detect IOCs over a time-sorted events slice.  RawLog is not needed.
// Assumes all of the events in the slice were accepted (properly formatted) and are part of the same ingest, with no duplicate line numbers
func BruteForceDetector(events []db.Event) []db.Detection {
	detections := []db.Detection{}
	ingestID := events[0].IngestID

	loginEventsWithUsers := FilterEvents(events, func(e db.Event) bool {
		return e.User != "" && slices.Contains(LoginEvents, e.Action)
	})

	for user, userEvents := range SplitBy(loginEventsWithUsers, db.Event.UserKey) {
		for _, bucket := range FailureBuckets(userEvents) {
			if len(bucket) <= bruteForceThresh {
				continue
			}

			severity := min(10*EventRate(bucket)/bruteForceRate, 100)
			confidence := min(30+20*math.Log10(float64(len(bucket))), 100)

			detections = append(detections, db.Detection{
				IngestID:    ingestID,
				Description: "Brute Force Attempt",
				Severity:    int16(severity),
				Confidence:  int16(confidence),
				Entity:      user,
				Observed:    float32(len(bucket)),
				Threshold:   float32(bruteForceThresh),
				Events:      LineNumbers(bucket),
			})
		}
	}

	return detections
}
