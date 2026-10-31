package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventParseAllColumns(t *testing.T) {
	event := Event{
		RawLog: "1700000000,alice,10.0.0.1,10.0.0.2,443,/login,POST,201,allow,portal,128,512,75,medium",
	}

	headers := []string{"epochtime", "login", "cip", "sip", "srv_dport", "url", "reqmethod", "respcode", "action", "appname", "reqsize", "respdatasize", "riskscore", "threatseverity"}

	event.Parse(headers)

	assert.Empty(t, event.Errors)

	require.NotNil(t, event.Time)
	assert.True(t, event.Time.Equal(time.Unix(1700000000, 0)), "time %s", *event.Time)
	assert.Equal(t, "alice", event.User)
	assert.Equal(t, "10.0.0.1", event.ClientIP)
	assert.Equal(t, "10.0.0.2", event.ServerIP)
	require.NotNil(t, event.ServerPort)
	assert.Equal(t, 443, *event.ServerPort)
	assert.Equal(t, "/login", event.URL)
	assert.Equal(t, "POST", event.Method)
	require.NotNil(t, event.ResponseCode)
	assert.Equal(t, 201, *event.ResponseCode)
	assert.Equal(t, "allow", event.Action)
	assert.Equal(t, "portal", event.AppName)
	require.NotNil(t, event.BytesIn)
	assert.Equal(t, 128, *event.BytesIn)
	require.NotNil(t, event.BytesOut)
	assert.Equal(t, 512, *event.BytesOut)
	require.NotNil(t, event.RiskScore)
	assert.Equal(t, 75, *event.RiskScore)
	assert.Equal(t, "medium", event.Severity)
}

func TestEventParseOmitsOptionalColumns(t *testing.T) {
	event := Event{RawLog: "1700000000"}

	event.Parse([]string{"epochtime"})

	assert.Empty(t, event.Errors)

	require.NotNil(t, event.Time)
	assert.True(t, event.Time.Equal(time.Unix(1700000000, 0)), "time %s", *event.Time)
	assert.Empty(t, event.User)
	assert.Empty(t, event.ClientIP)
	assert.Empty(t, event.ServerIP)
	assert.Nil(t, event.ServerPort)
	assert.Empty(t, event.URL)
	assert.Empty(t, event.Method)
	assert.Nil(t, event.ResponseCode)
	assert.Empty(t, event.Action)
	assert.Empty(t, event.AppName)
	assert.Nil(t, event.BytesIn)
	assert.Nil(t, event.BytesOut)
	assert.Nil(t, event.RiskScore)
	assert.Empty(t, event.Severity)
}

func TestEventParseNullMarkers(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "none", value: "None"},
		{name: "na", value: "NA"},
		{name: "n slash a", value: "N/A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{RawLog: "1700000000,alice," + tt.value + "," + tt.value + "," + tt.value + "," + tt.value + "," + tt.value}

			event.Parse([]string{"epochtime", "login", "srv_dport", "respcode", "reqsize", "respdatasize", "riskscore"})

			assert.Empty(t, event.Errors)
			assert.Equal(t, "alice", event.User, "login present in row")
			assert.Nil(t, event.ServerPort)
			assert.Nil(t, event.ResponseCode)
			assert.Nil(t, event.BytesIn)
			assert.Nil(t, event.BytesOut)
			assert.Nil(t, event.RiskScore)
		})
	}
}

func TestEventParseStripsResponseCodeReason(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "code only", value: "200", want: 200},
		{name: "code with reason", value: "200 - OK", want: 200},
		{name: "code with space separated reason", value: "404 Not Found", want: 404},
		{name: "code with multiple spaces", value: "500  Internal Server Error", want: 500},
		{name: "code with long reason", value: "429 - Too Many Requests", want: 429},
		{name: "code with trailing space", value: "201 ", want: 201},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{RawLog: "1700000000," + tt.value}

			event.Parse([]string{"epochtime", "respcode"})

			assert.Empty(t, event.Errors)
			require.NotNil(t, event.ResponseCode, "response code for %q", tt.value)
			assert.Equal(t, tt.want, *event.ResponseCode, "response code for %q", tt.value)
		})
	}
}

func TestEventParseRiskScoreBoundaries(t *testing.T) {
	for _, want := range []int{0, 50, 100} {
		event := Event{RawLog: "1700000000," + itoa(want)}

		event.Parse([]string{"epochtime", "riskscore"})

		assert.Empty(t, event.Errors)
		require.NotNil(t, event.RiskScore)
		assert.Equal(t, want, *event.RiskScore)
	}
}

func TestEventParseRecordsErrors(t *testing.T) {
	tests := []struct {
		name    string
		header  []string
		rawLog  string
		wantErr string
	}{
		{
			name:    "malformed row",
			header:  []string{"epochtime", "login"},
			rawLog:  "\"1700000000,alice",
			wantErr: "failed to parse epochtime",
		},
		{
			name:    "invalid time",
			header:  []string{"epochtime"},
			rawLog:  "not-a-number",
			wantErr: "failed to parse epochtime",
		},
		{
			name:    "invalid server port",
			header:  []string{"epochtime", "srv_dport"},
			rawLog:  "1700000000,abc",
			wantErr: "invalid srv_dport",
		},
		{
			name:    "invalid response code",
			header:  []string{"epochtime", "respcode"},
			rawLog:  "1700000000,abc",
			wantErr: "invalid response code",
		},
		{
			name:    "response code with non-numeric prefix",
			header:  []string{"epochtime", "respcode"},
			rawLog:  "1700000000,OK - 200",
			wantErr: "invalid response code",
		},
		{
			name:    "invalid request size",
			header:  []string{"epochtime", "reqsize"},
			rawLog:  "1700000000,abc",
			wantErr: "invalid reqsize",
		},
		{
			name:    "invalid response data size",
			header:  []string{"epochtime", "respdatasize"},
			rawLog:  "1700000000,abc",
			wantErr: "invalid respdatasize",
		},
		{
			name:    "invalid risk score",
			header:  []string{"epochtime", "riskscore"},
			rawLog:  "1700000000,abc",
			wantErr: "invalid riskscore",
		},
		{
			name:    "risk score below range",
			header:  []string{"epochtime", "riskscore"},
			rawLog:  "1700000000,-1",
			wantErr: "invalid riskscore",
		},
		{
			name:    "risk score above range",
			header:  []string{"epochtime", "riskscore"},
			rawLog:  "1700000000,101",
			wantErr: "invalid riskscore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{RawLog: tt.rawLog}

			event.Parse(tt.header)

			assert.Contains(t, event.Errors, tt.wantErr)
		})
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}

	digits := make([]byte, 0, 3)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}

	return string(digits)
}
