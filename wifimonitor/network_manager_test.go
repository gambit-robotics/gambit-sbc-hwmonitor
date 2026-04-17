package wifimonitor

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNmcliConnectionList(t *testing.T) {
	output, err := os.ReadFile("testdata/nmcli_connection_show.txt")
	require.NoError(t, err)

	got, err := parseNmcliConnectionList(string(output))
	require.NoError(t, err)

	// Only wifi connections, in input order, with colon-in-name un-escaped.
	assert.Equal(t, []string{"HomeWifi", "Guest:Net", "OldSaved"}, got)
}

func TestParseNmcliConnectionList_Empty(t *testing.T) {
	got, err := parseNmcliConnectionList("")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestParseNmcliConnectionList_MalformedLineSkipped(t *testing.T) {
	// Lines with too few fields should be skipped, not error the whole call.
	in := "OnlyOneField\nGoodOne:uuid:802-11-wireless:wlan0\n"
	got, err := parseNmcliConnectionList(in)
	require.NoError(t, err)
	assert.Equal(t, []string{"GoodOne"}, got)
}

func TestValidateProfileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"plain", "HomeWifi", false},
		{"spaces", "My Home Network", false},
		{"colon", "Guest:Net", false},
		{"empty", "", true},
		{"leading dash", "-rf", true},
		{"newline", "Home\nother", true},
		{"null byte", "Home\x00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProfileName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
