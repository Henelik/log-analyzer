package worker

import (
	"math"
	"slices"

	"github.com/Henelik/log-analyzer/db"
)

const (
	bruteForceThresh = 10
	bruteForceRate   = 1.0 / 60 // the number of brute force events per second to be considered a detection; one per minute

	portScanThresh    = 15
	exfiltraionThresh = 10 << 6 // egress events over 10 MB are flaggeed

	ActionLoginFail    = "Login Fail"
	ActionLoginSuccess = "Login Success"

	ActionEgress = "Egress"
)

type Detector func([]db.Event) []db.Detection

var Detectors = []Detector{
	BruteForceDetector,
	PortScanDetector,
}

var LoginEvents = []string{ActionLoginFail, ActionLoginSuccess}

// detect brute force IOCs over a time-sorted events slice.  RawLog is not needed.
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

			severity := min(10*EventRate(bucket)/bruteForceRate+10, 100)
			confidence := min(30+20*math.Log10(float64(len(bucket))), 100)
			priority := severity * (0.5 + 0.5*confidence/100.0)

			detections = append(detections, db.Detection{
				IngestID:    ingestID,
				Description: "Brute Force Attempt",
				Severity:    int16(severity),
				Confidence:  int16(confidence),
				Priority:    int16(priority),
				Entity:      user,
				Observed:    float32(len(bucket)),
				Threshold:   float32(bruteForceThresh),
				Events:      LineNumbers(bucket),
			})
		}
	}

	return detections
}

// detect port scan IOCs over a time-sorted events slice.  RawLog is not needed.
// Assumes all of the events in the slice were accepted (properly formatted) and are part of the same ingest, with no duplicate line numbers
func PortScanDetector(events []db.Event) []db.Detection {
	detections := []db.Detection{}
	ingestID := events[0].IngestID

	eventsWithPorts := FilterEvents(events, func(e db.Event) bool {
		return e.ServerPort != nil
	})

	for source, sourceEvents := range SplitBy(eventsWithPorts, db.Event.ClientIPKey) {
		if CountUnique(sourceEvents, db.Event.PortKey) >= portScanThresh {
			severity := min(float64(len(sourceEvents))/100.0, 100)
			confidence := min(30+20*math.Log10(float64(len(sourceEvents))), 100)
			priority := severity * (0.5 + 0.5*confidence/100.0)

			detections = append(detections, db.Detection{
				IngestID:    ingestID,
				Description: "Port Scan",
				Severity:    int16(severity),
				Confidence:  int16(confidence),
				Priority:    int16(priority),
				Entity:      source,
				Observed:    float32(len(sourceEvents)),
				Threshold:   float32(portScanThresh),
				Events:      LineNumbers(sourceEvents),
			})
		}
	}

	return detections
}

// detect singular data exfiltraion IOCs over a time-sorted events slice.  RawLog is not needed.
// Assumes all of the events in the slice were accepted (properly formatted) and are part of the same ingest, with no duplicate line numbers
func ExfilDetector(events []db.Event) []db.Detection {
	detections := []db.Detection{}
	ingestID := events[0].IngestID

	egressEvents := FilterEvents(events, func(e db.Event) bool {
		return e.BytesOut != nil && *e.BytesOut >= exfiltraionThresh && e.Action == ActionEgress
	})

	for _, event := range egressEvents {
		bytesOut := *event.BytesOut

		severity := min(float64(bytesOut)/exfiltraionThresh, 100)
		confidence := min(30+20*math.Log10(float64(bytesOut)/100000), 100)
		priority := severity * (0.5 + 0.5*confidence/100.0)

		entity := event.User
		if entity == "" {
			entity = event.ClientIP
		}

		detections = append(detections, db.Detection{
			IngestID:    ingestID,
			Description: "Data Exfiltration",
			Severity:    int16(severity),
			Confidence:  int16(confidence),
			Priority:    int16(priority),
			Entity:      entity,
			Observed:    float32(bytesOut),
			Threshold:   float32(portScanThresh),
			Events:      []int32{event.LineNumber},
		})
	}
}
