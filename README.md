# log-analyzer

## Data processing approach

### Data quality

Uploaded files are rejected with specific error messages if the data is invalid:
* Mismatched header/row length
* Missing epochtime column

Once the file is accepted, the events are parsed by a worker.  Any errors encountered at this stage are persisted to the database for review by an analyst.

### Normalization

"" (empty field), "NA", "N/A", and "None" are normalized to NULL in the database.  NULL values are JSON null (for ints) or an empty string (for strings) in API resonses.

## Design considerations

### Epoch time is required

Epoch time is the only fully unambiguous way to handle event timing.  I considered also supporting time+tz, but that involved more sophisticated checks when uploading a file, as well as mapping 3-letter time zone codes to UTC offsets, which is complicated and ambiguous for international event ingests.  I decided to keep it simple for this project.

### Streaming file upload

The uploaded file is separated line-by-line and streamed to the database in batches of 1000 events at a time to reduce memory usage and avoid hitting SQL query size limits.  This is done in a transaction so if the file is found to be malformed at the last line, everything is rolled back.

### Events are parsed out-of-band

Events are parsed asynchronously after the file upload is completed.  This approach allows for larger files to be uploaded without hitting memory/timeout limits.

### Event parsing surfaces all errors

When errors are discovered during event parsing, the error is added to a log and parsing continues.  That way, all issues with an event's data are flagged for analyst review, and valid data is still processed for partially-valid events.  The event is flagged as "rejected" and not included in analysis.

## IOC Detections

### Brute force

Large number of failures per user/IP address in a short window, followed by success.

### Port scan

Requests to many ports from a single source.

### Data exfiltration

Large egress activity to an external location.

## Problems encountered during development

### CSV parsing

I originally implemented uploaded file paring using the first-party `encoding/csv` package.  The package conveneintly allows iterating over the records of a CSV while automatically splitting each row into a `[]string`, and un-quoting quoted fields.  I saved the raw logs for each event by joining the return string slice with commas, using `strings.Join(record, ",")`.  

However, this can result in malformed fields because the conversion back to a single string doesn't account for quoted fields with commas.  For example:

```
Original row:

"meow_mix,\"abcd,efgh\",doggy_dinner"

Row returned by csv.Reader.Read():

[]string{"meow_mix", "abcd,efgh", "doggy_dinner"}

Saved raw log (4 comma-separated values instead of 3):

"meow_mix,abcd,efgh,doggy_dinner"
```

I solved this problem by scanning the incoming file line-by-line using `bufio.Scanner` instead of `csv.Reader`, saving the raw line to the Event object verbatim for later processing.

To ensure the file was not malformed, I still used a `csv.Reader` to count the number of values in each record of the file.  A more efficient solution would use a custom function to count the number of commas in the row string, while ignore commas inside quoted fields.

### Over-complicated detector

On first pass, I implemented a detector that walked the ordered event list and categorized relevant event indeces into maps keyed by user or other relevant data types.  After finishing the first detector, I found that the code was difficult to modify or reason about, so I changed my approach to re-usable functions that filter event slices by attributes.  This allowed simpler logic, as the detector algorithm could walk a single event stream pre-filtered for relevance, e.g. login events for one user.  The tradeoff is that filtering is theoreticall less memory efficient, but more readable.

## Future enhancements (outside of scope)

### Multi-tenancy

To achieve multi-tenancy, I would tag all files, events, detections, and users with a tenant ID, which would be part of their composite primary key.  An ABAC layer in the API would ensure customer users can only view their own data, while analysts can view data from all tenants.

### Event stream ingest

This app is built only to handle file uploads, but most businesses want event streams to be ingested and alerted on.  The detection engine would also need to be updated to buffer out-of-order events, and correlate late events to existing detections.

### User-configurable detectors

Detection thresholds, time windows, etc. for detections could be made configurable either via a structured config file, or via web UI and stored in the database.  If the latter, custom detectors could also be made available per-tenant.

### Detector architecture

Right now, IOC detection and event analysis only happens within the scope of a single ingest (file upload).  If two separate ingests contain part of an IOC pattern, the IOC would not be detected.

I would implement cross-ingest analysis by querying events from the same time window, with the same client/server IP, or the same user, etc. so that relevant information is included in the analysis regardless of its source.

### Detector threshold tuning

Detector threshold values were written without solid context, and should be revisited and adjusted based on SOC feedback and production data.

### Asynchronous analysis workers

In the current implementation, all analysis of events is done with the entire valid portion of the ingested file in memory, and detections are only created in the database after the full analysis is completed.  I chose to implement it this way for simplicity, but it is a risky approach for a production app because any failure in any detection or the database connection would result in missed or delayed detections.

Ideally, each type of detector would query only the data it needs from the database, and run in a separate process, independently of other detection logic.

### Pagination on GET endpoints

Large files will potentially generate large numbers of errors and detections.  Pagination would help users navigate those objects without a slow user experience.

