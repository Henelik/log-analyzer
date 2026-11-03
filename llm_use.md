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
| Event filtering functions | backend/worker/detectors.go:44-62 Add a loop to step backward through failureEvents to find the earliest failure within the brute force window, and set the starting index of the relevantEvents slice to that. | Deepseek v4.1 Flash | Ollama Cloud |
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
| Implement errors endpoint | Implement the errors endpoint per @backend/rest/register.go , which returns the line number, raw log, and error string for each event with the status EventStatusRejected associated with the given file. | Deepseek v4.1 Flash | Ollama Cloud |
| Implement detections endpoint | Implement the detections endpoint per @backend/rest/register.go that returns a list of detections for a given file ID. | Deepseek v4.1 Flash | Ollama Cloud |
| Auth middleware | Implement Fiber middleware that restricts access to all API endpoints (except /login) to authenticated users with valid JWT tokens signed by this server. | Deepseek v4.1 Flash | Ollama Cloud |
| User seeding | Create a function that seeds a user in the database.  @main.go should call the function to set up a test user if credentials were provided via environment variables, after migrating the database. | Deepseek v4.1 Flash | Ollama Cloud |
| Log generator script | Create a Python script that generates an event log CSV file with a specified number of events and IOCs, defined via env.  The file should follow the format in @db/event.go .  The script should generate values for all parsed fields.  Invalid events should be peppered in at a ratio defined via env.  Events may be invalid because of missing or incorrect data in epochtime, srv_dport, respcode, reqdatasize, or respdatasize.  Events may also be invalid because of a field number mismatch between that event and the file header. | Deepseek v4.1 Flash | Ollama Cloud |
|  | Event timestamps should not be incrementing, they should be a random number of seconds apart from 0 to 100. |  |  |
|  | Add a new feature that makes some rows duplicates of a neighboring row, definted by the DUPLICATE_RATIO environment variable (default 0.05).  Rows that were already invalidated should not be selected to become duplicate rows. |  |  |
|  | IOCs generated should have events 1 to 100 times the threshold, rather than exacatly the threshold every time. |  |  |
| Error table | In @frontend/src/components/ErrorList.svelte , rejected rows that have the same item count as the headers need to be shown in a table with the header from the file object, instead of simply being displayed as a raw log. | Deepseek v4.1 Flash | Ollama Cloud |
| Deduplication | Implement deduplication in @rest/upload.go , using SHA256 to hash raw rows into keys, and then check if the row was already parsed before inserting it.  If it is already present, increment the counter on the file object. | Deepseek v4.1 Flash | Ollama Cloud |
|  | The row hashes should be stored in-memory only, not checked against the database.  Deduplication is per-file only. |  |  |
| Add CountUnique function | In @worker/events.go , add a function to count the number of unique values given by a function among events, e.g. the number of unique users or client IPs. | Deepseek v4.1 Flash | Ollama Cloud |
| Add port scan event generation to script | Update @scripts/generate_events.py to generate port scan IOCs as well as brute force IOCs.  The number of each type of IOC should be independently controlled. | Deepseek v4.1 Flash | Ollama Cloud |
|  | The script currently can generate brute force IOCs in the background events.  The code that generates background events should not generate more than .5 the threshold of IOC events; successive login failures in the case of brute force attacks. |  |  |
| Add port to eval detection event table | The port needs to be displayed in the event table for IOC detections in the UI. | Deepseek v4.1 Flash | Ollama Cloud |
| AI detector description worker | Create a new worker in @worker/ that queries the database for detections with no description, and makes a call to the Ollama chat API to get a description for each detection.  The result of the Ollama call must be JSON and follow a strict schema.  If the Ollama call fails, log the error and skip the rest of the detections in this tick.  If the Ollama API returns an invalid response, log the error and update that detection's description to "AI summary returned invalid". | Deepseek v4.1 Flash | Ollama Cloud |
|  | The description prompt should include all of the relevant entities in TOON format.  Exclude the raw logs, status, errors, ingest IDs, and line numbers. |  |  |
|  | The worker needs to update the detection notes rather than the description.  It should query based on the notes field, not description. |  |  |
|  | The model is including Markdown style code quoting in the response.  Before trying to parse the JSON response, check for and remove the code quoting. |  |  |
| Time range in detector dropdown | The UI should display the time range of each detection, as determined by the earliest and latest event times in the relevant events.  The time display should be inside the dropdown for the detection. @frontend/src/components/DetectionList.svelte | Deepseek v4.1 Flash | Ollama Cloud |
|  | The detection description should be displayed after the detection metadata and above the event table in the dropdown. |  |  |