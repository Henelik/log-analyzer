# LLM Use

Prompts and commands used during development of this app.

Kilo Code was used as the harness for all interactions.

| Session | Prompt | Model | Provider |
| -- | -- | -- | -- |
| DB setup | Finish implementing the DB setup function in @db.go. | Deepseek v4.1 Flash | Ollama Cloud |
| Health check setup | Implement the /healthz and /readyz handlers in @rest. | Deepseek v4.1 Flash | Ollama Cloud |
|  | Use the testify assert and require packages in the test code. |  |  |
|  | The SQLite database setup helper for tests should be provided by the db package. |  |  |
| Upload handler initial implementation | Finish implementing the file upload handler in @backend/rest/upload.go .  The handler should accept a CSV file as multipart form data, create a file object in the database per @backend/db/file.go .  The upload handler should only add values to the Filename, Bytes, Status, and Raw fields.  Everything else should be kept at zero-value for out-of-band processing. | Deepseek v4.1 Flash | Ollama Cloud |
| UploadFile endpoint test creation | Create tests for the UploadFIle endpoint in @backend/rest/upload.go .  Cover the inclusion and omission of every field in the File model, as well as valid and invald values where applicable. | Deepseek v4.1 Flash | Ollama Cloud |
|  | Review the tests in @backend/rest/upload_test.go for thoroughness and correctness. | GLM 5.3 |  |
|  | Add a test to cover multiple files uploaded to a single DB, and add assertions to ensure events are related to the uploaded files. | Deepseek v4.1 Flash |  |
|  | Add tests to ensure response code strings such as "200 - OK" are properly stripped before being parsed as an int. | Deepseek v4.1 Flash |  |
|  | Update the test cases to reflect the design change in the UploadFIle endpoint.  Event field parsing is now done out-of-band. | Deepseek v4.1 Flash |  |
| Files get/delete implementaiton | Implement the get and delete handlers for for files in @backend/rest/register.go .  Ignore analysis, errors, and events for now. | Deepseek v4.1 Flash | Ollama Cloud |
| Event parse worker implementation | Finish implementing the event parser in @backend/worker/parser.go .  It needs to query files with FilesStatusUploaded, then query batches of events for the each file, parse them, and persist their new state.  Once all of the events for a file are parse, the file's status should be updated. | Deepseek v4.1 Flash | Ollama Cloud |
| Event filtering functions | backend/worker/detectors.go:44-62
```
if window <= bruteForceWindow {
						// tuned confidence equation
						// 5 attempts = 20 confidence
						// 25+ events = 100 confidence
						confidence := min(len(failureEvents)*20/bruteForceSuccessThresh, 100)

						relevantEvents := failureEvents[len(failureEvents)-bruteForceSuccessThresh-1:]
						relevantEvents = append(relevantEvents, event.LineNumber)

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
```

Add a loop to step backward through failureEvents to find the earliest failure within the brute force window, and set the starting index of the relevantEvents slice to that. | Deepseek v4.1 Flash | Ollama Cloud |
|  | Make the relevant event search a re-usable function.  Also add a function that can be used to filter the events using a function. |  |  |
|  | Add a function to get a list of all users who appear in any event in a slice. |  |  |
|  | Create a SplitBy function that splits a slice of events by the event attribute returned by a function, and returns a map where each key is the attribute's value and the value is a slice of events for that user. |  |  |
|  | Add a function to calculate the average rate of events between the first and last event in a slice, inclusive. |  |  |
|  | Update the BruteForceDetector to bucket consecutive failures into windows preceding successes, per user.  Over `bruteForceThresh` failures before any success will be detected as brute force.  If there were no successes for a user, all of the failures for that user should be in a single bucket. |  |  |
|  | Make a re-usable function to convert an event slice into a slice of line numbers. |  |  |
|  | Add tests covering all of the functions in @backend/worker/events.go .  Cover all edge cases and documented behaviors. |  |  |
| Analysis worker implementation | Finish implementing the analysis worker in @backend/worker/analyzer.go .  It needs to query the database for files with FileStatusParsed, then for each file, query all of the file's events, omitting RawLog, sorted by time, and then run all of the detectors in the Detectors slice in @backend/worker/detectors.go on them.  The resulting detections then need to be inserted into the database. | Deepseek v4.1 Flash | Ollama Cloud |
| Events POST handler | Implement the events POST handler per @backend/rest/register.go .  This is a read-only endpoint.  The client submits a JSON list of line numbers for events to retrieve. | Deepseek v4.1 Flash | Ollama Cloud |
| Login handler implementation | Implement the login handler per @backend/rest/register.go .  Clients submit a username and password via multipart form data. The handler should validate the credentials and return a signed JWT token if the credentials are correct. | Deepseek v4.1 Flash | Ollama Cloud |
| DB migration function | Create a database table migration function in @backend/db/db.go and call it after connecting to the database. | Deepseek v4.1 Flash | Ollama Cloud |