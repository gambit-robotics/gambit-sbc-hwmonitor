package wifimonitor

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.viam.com/rdk/logging"
)

// Separate from wifi_linux_test.go so these validation tests run on any
// platform (the linux-tagged file doesn't compile on macOS).

type recordingNetworkManager struct {
	forgottenName string
}

func (m *recordingNetworkManager) ListSavedNetworks() ([]string, error) {
	return nil, nil
}

func (m *recordingNetworkManager) ForgetNetwork(name string) error {
	m.forgottenName = name
	return nil
}

func newForgetTestConfig(t *testing.T, nm WifiNetworkManager) *Config {
	return &Config{
		mu:             sync.Mutex{},
		logger:         logging.NewTestLogger(t),
		networkManager: nm,
	}
}

// A name starting with '-' could be interpreted as a flag by nmcli — e.g.
// `nmcli connection delete -h` would print help instead of failing, and
// future nmcli flags could be more dangerous. Reject before exec.
func TestDoCommandForgetNetworkRejectsFlagInjection(t *testing.T) {
	mock := &recordingNetworkManager{}
	c := newForgetTestConfig(t, mock)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "-rf",
	})
	assert.Error(t, err)
	assert.Empty(t, mock.forgottenName, "must not invoke nmcli with a flag-like name")
}

func TestDoCommandForgetNetworkRejectsControlChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"newline", "Home\nOther"},
		{"carriage_return", "Home\rOther"},
		{"null_byte", "Home\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &recordingNetworkManager{}
			c := newForgetTestConfig(t, mock)
			_, err := c.DoCommand(context.Background(), map[string]interface{}{
				"command": "forget_network",
				"name":    tt.input,
			})
			assert.Error(t, err)
			assert.Empty(t, mock.forgottenName, "must not invoke nmcli with a name containing control chars")
		})
	}
}
