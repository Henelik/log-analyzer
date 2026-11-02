package csvutil

import (
	"encoding/csv"
	"strings"

	"github.com/pkg/errors"
)

// split a single CSV row into values.  If multiple rows are in the "row" string, only values from the first row are returned.
func ParseRow(row string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(row))

	return reader.Read()
}

// count the number of values in a CSV row.  If multiple rows are in the "row" string, only the first row will be counted.
func CountRowValues(row string) (int, error) {
	values, err := ParseRow(row)

	return len(values), errors.Wrap(err, "failed to count CSV row values")
}
