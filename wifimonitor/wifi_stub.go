//go:build !linux && !windows

// Test-only stubs so this package compiles on macOS/other. The production
// module only runs on Linux (see config.go Validate); these stubs let the
// platform-independent parse and dispatch logic be tested locally.

package wifimonitor

func (c *Config) newWifiMonitor(adapter string) WifiMonitor       { return nil }
func (c *Config) newNetworkProfileManager() networkProfileManager { return nil }
