//go:build !linux && !windows

// Stubs so this package compiles on macOS/other for local test runs. The
// production module only runs on Linux (see config.go Validate).

package wifimonitor

import "go.viam.com/rdk/logging"

func (c *Config) newWifiMonitor(adapter string) WifiMonitor      { return nil }
func newNetworkManager(logger logging.Logger) WifiNetworkManager { return nil }
