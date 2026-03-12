package wifimonitor

import "errors"

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
