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

## IOC Detections

### Brute force

Large number of failures per user/IP address in a short window, followed by success.

### Impossible travel

Requests from the same user from two separate locations, withouth enough time to travel between them.

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

