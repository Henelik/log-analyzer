package worker

import (
	"errors"
	"time"

	"github.com/Henelik/log-analyzer/db"
)

const (
	// purposely sensitive.  5 legitimate failures in a row is fairly unlikely, and exactly 5 failures generates a detection with low confidence
	bruteForceSuccessThresh = 5 // number of failures preceding a success to be considered brute force
	bruteForceWindow        = time.Minute * 5
	bruteForceFailureThresh = 10 // number of successive failures in the window to be considered brute force
)

type DetectorCooldown struct {
	EventTime  time.Time
	EventIndex int
}

// detect ICOs over a time-sorted events slice.  RawLog is not needed.
// Assumes all of the events in the slice were accepted (properly formatted) and are part of the same ingest (no duplicate line numbers)
func BruteForceDetector(events []db.Event) ([]db.Detection, error) {
	// map user: event line numbers
	failureEventMap := map[string][]int32{}
	detections := []db.Detection{}
	var lastTime time.Time

	for _, event := range events {
		if lastTime.Before(event.Time) || lastTime.Equal(event.Time) {
			lastTime = event.Time
		} else {
			return nil, errors.New("events are not sorted with ascending time")
		}

		if event.User == "" {
			continue
		}

		switch event.Action {
		case "Login Fail":
			failureEventMap[event.User] = append(failureEventMap[event.User], event.LineNumber)

			failureEvents := failureEventMap[event.User]

			if len(failureEvents) >= bruteForceFailureThresh {
				window := event.Time.Sub(events[failureEvents[len(failureEvents)-bruteForceSuccessThresh]].Time)

				if window <= bruteForceWindow {
					relevantEvents := windowLineNumbers(events, failureEvents, event.Time, bruteForceWindow, event.LineNumber)

					confidence := min(len(relevantEvents)*10/bruteForceFailureThresh+35, 95)

					detections = append(detections, db.Detection{
						Description: "Brute Force Attempt",
						Severity:    50,
						Confidence:  int16(confidence),
						Entity:      event.User,
						Observed:    float32(len(failureEvents)),
						Threshold:   float32(bruteForceSuccessThresh),
						Events:      relevantEvents,
					})
				}
			}
		case "Login Success":
			if failureEvents, ok := failureEventMap[event.User]; ok {
				if len(failureEvents) >= bruteForceSuccessThresh {
					window := event.Time.Sub(events[failureEvents[len(failureEvents)-bruteForceSuccessThresh]].Time)

					if window <= bruteForceWindow {
						relevantEvents := windowLineNumbers(events, failureEvents, event.Time, bruteForceWindow, event.LineNumber)

						confidence := min(len(relevantEvents)*20/bruteForceSuccessThresh, 95)

						detections = append(detections, db.Detection{
							Description: "Brute Force Login",
							Severity:    100,
							Confidence:  int16(confidence),
							Entity:      event.User,
							Observed:    float32(len(failureEvents)),
							Threshold:   float32(bruteForceSuccessThresh),
							Events:      relevantEvents,
						})
					}
				}
			}
		}
	}

	return detections, nil
}
