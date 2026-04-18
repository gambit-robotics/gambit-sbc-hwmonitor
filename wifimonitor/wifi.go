package wifimonitor

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotConnected      = errors.New("not connected to a network")
	ErrAdapterNotFound   = errors.New("adapter not found")
	ErrNoAdaptersFound   = errors.New("no adapters found")
	ErrNmcliNotAvailable = errors.New("nmcli is not available on this system")
)

type WifiMonitor interface {
	GetNetworkStatus() (*networkStatus, error)
}

type WifiNetworkManager interface {
	ListSavedNetworks() ([]string, error)
	ForgetNetwork(name string) error
}

// validateProfileName rejects names that would be unsafe to pass to
// `nmcli connection delete` as a positional argument. NetworkManager profile
// names can legally contain spaces, unicode, and colons, but never newlines
// or null bytes; a leading '-' could be parsed as a flag.
func validateProfileName(name string) error {
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("profile name %q starts with '-' and could be interpreted as a flag", name)
	}
	if strings.ContainsAny(name, "\n\r\x00") {
		return errors.New("profile name contains control character")
	}
	return nil
}

type networkStatus struct {
	NetworkName       string
	SignalStrength    int
	TxSpeedMbps       float64
	RxSpeedMbps       float64
	FrequencyMHz      int
	TxRetries         int
	TxFailed          int
	BeaconSignalAvg   int
	SignalAvg         int
	AckSignalAvg      int
	Noise             int
	ConnectedTimeSec  int
	InactiveTimeMs    int
}
