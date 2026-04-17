package wifimonitor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.viam.com/rdk/logging"
)

type fakeProfileManager struct {
	listOut   []string
	listErr   error
	forgotten []string
	forgetErr error
}

func (f *fakeProfileManager) ListSavedNetworks() ([]string, error) {
	return f.listOut, f.listErr
}

func (f *fakeProfileManager) ForgetNetwork(name string) error {
	if f.forgetErr != nil {
		return f.forgetErr
	}
	f.forgotten = append(f.forgotten, name)
	return nil
}

func newTestConfig(mgr networkProfileManager) *Config {
	return &Config{
		logger:         logging.NewTestLogger(&testing.T{}),
		profileManager: mgr,
	}
}

func TestDoCommand_ListSavedNetworks(t *testing.T) {
	mgr := &fakeProfileManager{listOut: []string{"A", "B"}}
	c := newTestConfig(mgr)

	res, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "list_saved_networks",
	})
	require.NoError(t, err)

	nets, ok := res["networks"].([]string)
	require.True(t, ok, "expected networks to be []string, got %T", res["networks"])
	assert.Equal(t, []string{"A", "B"}, nets)
}

func TestDoCommand_ListSavedNetworks_ManagerError(t *testing.T) {
	mgr := &fakeProfileManager{listErr: errors.New("nmcli missing")}
	c := newTestConfig(mgr)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "list_saved_networks",
	})
	assert.Error(t, err)
}

func TestDoCommand_ForgetNetwork(t *testing.T) {
	mgr := &fakeProfileManager{}
	c := newTestConfig(mgr)

	res, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "HomeWifi",
	})
	require.NoError(t, err)
	assert.Equal(t, true, res["forgotten"])
	assert.Equal(t, []string{"HomeWifi"}, mgr.forgotten)
}

func TestDoCommand_ForgetNetwork_MissingName(t *testing.T) {
	mgr := &fakeProfileManager{}
	c := newTestConfig(mgr)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
	})
	assert.Error(t, err)
	assert.Empty(t, mgr.forgotten)
}

func TestDoCommand_ForgetNetwork_InvalidName(t *testing.T) {
	mgr := &fakeProfileManager{}
	c := newTestConfig(mgr)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "-rf",
	})
	assert.Error(t, err)
	assert.Empty(t, mgr.forgotten, "must not call ForgetNetwork with unsafe name")
}

func TestDoCommand_ForgetNetwork_ManagerError(t *testing.T) {
	mgr := &fakeProfileManager{forgetErr: errors.New("connection not found")}
	c := newTestConfig(mgr)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "forget_network",
		"name":    "Ghost",
	})
	assert.Error(t, err)
}

func TestDoCommand_UnknownCommand(t *testing.T) {
	mgr := &fakeProfileManager{}
	c := newTestConfig(mgr)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "nuke_from_orbit",
	})
	assert.Error(t, err)
}

func TestDoCommand_MissingCommand(t *testing.T) {
	mgr := &fakeProfileManager{}
	c := newTestConfig(mgr)

	_, err := c.DoCommand(context.Background(), map[string]interface{}{})
	assert.Error(t, err)
}

func TestDoCommand_NoManagerConfigured(t *testing.T) {
	// When nmcli is missing on the host, profileManager is nil.
	// DoCommand must return a clear error, not panic.
	c := &Config{logger: logging.NewTestLogger(&testing.T{})}

	_, err := c.DoCommand(context.Background(), map[string]interface{}{
		"command": "list_saved_networks",
	})
	assert.Error(t, err)
}
