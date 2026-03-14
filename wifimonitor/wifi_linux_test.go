package wifimonitor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.viam.com/rdk/logging"
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

func TestParseConnectionList(t *testing.T) {
	output, err := os.ReadFile("testdata/nmcli_connections.txt")
	require.NoError(t, err)

	m := &nmcliNetworkManager{}
	networks := m.parseConnectionList(string(output))

	assert.Equal(t, []string{"HomeWiFi", "OfficeWiFi", "MobileHotspot"}, networks)
}

func TestParseConnectionListEmpty(t *testing.T) {
	m := &nmcliNetworkManager{}
	networks := m.parseConnectionList("")
	assert.Empty(t, networks)
}

func TestParseConnectionListNoWifi(t *testing.T) {
	m := &nmcliNetworkManager{}
	networks := m.parseConnectionList("Wired connection 1:802-3-ethernet\nlo:loopback")
	assert.Empty(t, networks)
}

func TestParseConnectionListColonInName(t *testing.T) {
	m := &nmcliNetworkManager{}
	// nmcli -t escapes colons in values as \:
	networks := m.parseConnectionList(`My\:Network:802-11-wireless`)
	assert.Equal(t, []string{"My:Network"}, networks)
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

// Mock network manager for DoCommand tests
type mockNetworkManager struct {
	networks      []string
	forgetErr     error
	forgottenName string
}

func (m *mockNetworkManager) ListSavedNetworks() ([]string, error) {
	return m.networks, nil
}

func (m *mockNetworkManager) ForgetNetwork(name string) error {
	m.forgottenName = name
	return m.forgetErr
}

// Mock wifi monitor for active network protection tests
type mockWifiMonitor struct {
	status *networkStatus
	err    error
}

func (m *mockWifiMonitor) GetNetworkStatus() (*networkStatus, error) {
	return m.status, m.err
}

func newTestConfig(t *testing.T, nm WifiNetworkManager) *Config {
	return &Config{
		mu:             sync.Mutex{},
		logger:         logging.NewTestLogger(t),
		networkManager: nm,
	}
}

func TestDoCommandListNetworks(t *testing.T) {
	mock := &mockNetworkManager{networks: []string{"HomeWiFi", "OfficeWiFi"}}
	c := newTestConfig(t, mock)

	result, err := c.DoCommand(context.Background(), map[string]interface{}{"command": "list_saved_networks"})
	require.NoError(t, err)
	assert.Equal(t, []string{"HomeWiFi", "OfficeWiFi"}, result["networks"])
}

func TestDoCommandForgetNetwork(t *testing.T) {
	mock := &mockNetworkManager{networks: []string{"HomeWiFi", "OfficeWiFi"}}
	c := newTestConfig(t, mock)

	result, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "OfficeWiFi",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
	assert.Equal(t, "OfficeWiFi", result["name"])
	assert.Equal(t, "OfficeWiFi", mock.forgottenName)
}

func TestDoCommandForgetNetworkError(t *testing.T) {
	mock := &mockNetworkManager{forgetErr: errors.New("connection 'BadNet' not found")}
	c := newTestConfig(t, mock)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "BadNet",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "BadNet")
}

func TestDoCommandUnknownCommand(t *testing.T) {
	c := newTestConfig(t, &mockNetworkManager{})

	_, err := c.DoCommand(context.Background(), map[string]interface{}{"command": "invalid"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

func TestDoCommandMissingCommand(t *testing.T) {
	c := newTestConfig(t, &mockNetworkManager{})

	_, err := c.DoCommand(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing or invalid 'command' field")
}

func TestDoCommandNoNetworkManager(t *testing.T) {
	c := newTestConfig(t, nil)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{"command": "list_saved_networks"})
	assert.ErrorIs(t, err, ErrNmcliNotAvailable)

	_, err = c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "SomeNet",
	})
	assert.ErrorIs(t, err, ErrNmcliNotAvailable)
}

func TestDoCommandForgetMissingName(t *testing.T) {
	c := newTestConfig(t, &mockNetworkManager{})

	_, err := c.DoCommand(context.Background(), map[string]interface{}{"command": "forget_network"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing or invalid 'name' parameter")
}

func TestDoCommandForgetEmptyName(t *testing.T) {
	c := newTestConfig(t, &mockNetworkManager{})

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network name cannot be empty")
}

func TestDoCommandForgetActiveNetworkReturnsWarning(t *testing.T) {
	mock := &mockNetworkManager{}
	c := newTestConfig(t, mock)
	c.wifiMonitor = &mockWifiMonitor{status: &networkStatus{NetworkName: "HomeWiFi"}}

	result, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "HomeWiFi",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
	assert.Equal(t, "HomeWiFi", mock.forgottenName)
	assert.Contains(t, result["warning"], "active network")
}

func TestDoCommandForgetInactiveNetworkNoWarning(t *testing.T) {
	mock := &mockNetworkManager{}
	c := newTestConfig(t, mock)
	c.wifiMonitor = &mockWifiMonitor{status: &networkStatus{NetworkName: "HomeWiFi"}}

	result, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "OfficeWiFi",
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result["status"])
	_, hasWarning := result["warning"]
	assert.False(t, hasWarning)
}

func TestReadingsIncludesSavedNetworks(t *testing.T) {
	mock := &mockNetworkManager{networks: []string{"HomeWiFi", "OfficeWiFi"}}
	c := newTestConfig(t, mock)
	c.wifiMonitor = &mockWifiMonitor{status: &networkStatus{
		NetworkName:   "HomeWiFi",
		SignalStrength: -44,
	}}

	readings, err := c.Readings(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "HomeWiFi", readings["network"])
	assert.Equal(t, []interface{}{"HomeWiFi", "OfficeWiFi"}, readings["saved_networks"])
}

func TestReadingsOmitsSavedNetworksWhenNoManager(t *testing.T) {
	c := newTestConfig(t, nil)
	c.wifiMonitor = &mockWifiMonitor{status: &networkStatus{NetworkName: "HomeWiFi"}}

	readings, err := c.Readings(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "HomeWiFi", readings["network"])
	_, hasSaved := readings["saved_networks"]
	assert.False(t, hasSaved)
	assert.Equal(t, true, readings["saved_networks_unavailable"])
}
