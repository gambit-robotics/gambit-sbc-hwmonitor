package wifimonitor

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinuxProcWifiMonitor(t *testing.T) {
	output, err := os.ReadFile("testdata/linux_proc.txt")
	assert.NoError(t, err)
	tests := []struct {
		name           string
		adapter        string
		signalStrength int
		linkSpeed      float64
		expectedError  error
	}{
		{"AdapterExists", "wlan0", -64, 46.0, nil},
		{"AdapterDoesNotExist", "wlan1", -1, -1, ErrAdapterNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &procWifiMonitor{adapter: tt.adapter}
			status, err := w.parseNetworkStatus(string(output))
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
				return
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.signalStrength, status.SignalStrength)
				assert.Equal(t, tt.linkSpeed, status.TxSpeedMbps)
			}
		})
	}
}

func TestLinuxIwWifiMonitor(t *testing.T) {
	tests := []struct {
		name           string
		adapter        string
		signalStrength int
		rxSpeed        float64
		txSpeed        float64
		frequency      int
		expectedError  error
		file           string
	}{
		{"AdapterExistsConnected", "wlan0", -65, 52.0, 72.2, 2412, nil, "iw_wlan0_connected.txt"},
		{"AdapterExistsNotConnected", "wlan0", -1, -1, -1, 0, ErrNotConnected, "iw_wlan0_not_connected.txt"},
		{"AdapterDoesNotExist", "wlan1", -1, -1, -1, 0, ErrAdapterNotFound, "iw_wlan1_does_not_exist.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := os.ReadFile(fmt.Sprintf("testdata/%v", tt.file))
			require.NoError(t, err)
			w := &iwWifiMonitor{adapter: tt.adapter}
			status, err := w.parseNetworkStatus(string(output))
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
				return
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.signalStrength, status.SignalStrength)
				assert.Equal(t, tt.rxSpeed, status.RxSpeedMbps)
				assert.Equal(t, tt.txSpeed, status.TxSpeedMbps)
				assert.Equal(t, tt.frequency, status.FrequencyMHz)
			}
		})
	}
}

func TestLinuxIwStationDump(t *testing.T) {
	output, err := os.ReadFile("testdata/iw_station_dump.txt")
	require.NoError(t, err)

	w := &iwWifiMonitor{adapter: "wlan0"}
	status := &networkStatus{}
	w.parseStationDump(string(output), status)

	assert.Equal(t, 123, status.TxRetries)
	assert.Equal(t, 5, status.TxFailed)
	assert.Equal(t, -62, status.BeaconSignalAvg)
	assert.Equal(t, -64, status.SignalAvg)
	assert.Equal(t, -63, status.AckSignalAvg)
	assert.Equal(t, 3600, status.ConnectedTimeSec)
	assert.Equal(t, 100, status.InactiveTimeMs)
}

func TestLinuxIwStationDumpMultiAntenna(t *testing.T) {
	// Test multi-antenna format: "signal avg: -49 [-57, -56, -54] dBm"
	output := `Station a1:b2:c3:d4:e5:f6 (on wlan0)
	signal avg:	-49 [-57, -56, -54] dBm
	beacon signal avg:	-50 dBm
	ack signal avg:	-48 dBm
`
	w := &iwWifiMonitor{adapter: "wlan0"}
	status := &networkStatus{}
	w.parseStationDump(output, status)

	assert.Equal(t, -49, status.SignalAvg)
	assert.Equal(t, -50, status.BeaconSignalAvg)
	assert.Equal(t, -48, status.AckSignalAvg)
}

func TestLinuxIwSurveyDump(t *testing.T) {
	output := `Survey data from wlan0
	frequency:			2412 MHz
	noise:				-92 dBm
	channel active time:		1234 ms
	channel busy time:		567 ms
Survey data from wlan0
	frequency:			2437 MHz [in use]
	noise:				-95 dBm
	channel active time:		5678 ms
	channel busy time:		890 ms
Survey data from wlan0
	frequency:			2462 MHz
	noise:				-90 dBm
`
	w := &iwWifiMonitor{adapter: "wlan0"}

	// Test with [in use] marker
	status := &networkStatus{}
	w.parseSurveyDump(output, status)
	assert.Equal(t, -95, status.Noise)

	// Test matching by frequency
	status2 := &networkStatus{FrequencyMHz: 2412}
	w.parseSurveyDump(output, status2)
	assert.Equal(t, -92, status2.Noise)
}

func TestLinuxNmcliWifiMonitor(t *testing.T) {
	output, err := os.ReadFile("testdata/nmcli.txt")
	assert.NoError(t, err)
	tests := []struct {
		name           string
		adapter        string
		signalStrength int
		linkSpeed      float64
		expectedError  error
	}{
		{"AdapterExists", "wlan0", -55, 195.0, nil},
		{"AdapterExistsNotConnected", "wlan2", -1, -1, ErrNotConnected},
		{"AdapterDoesNotExist", "wlan1", -1, -1, ErrAdapterNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &nmcliWifiMonitor{adapter: tt.adapter}
			status, err := w.parseNetworkStatus(string(output))
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
				return
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.signalStrength, status.SignalStrength)
				assert.Equal(t, tt.linkSpeed, status.TxSpeedMbps)
			}
		})
	}
}
