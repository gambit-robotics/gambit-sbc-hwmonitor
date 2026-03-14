package wifimonitor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.viam.com/rdk/components/sensor"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"

	"github.com/rinzlerlabs/viam-sbc-hwmonitor/utils"
)

var (
	Model       = resource.NewModel(utils.Namespace, "hwmonitor", "wifi_monitor")
	API         = sensor.API
	PrettyName  = "WiFi Monitor Sensor"
	Description = "A sensor that reports the status of the WiFi connection"
	Version     = utils.Version
)

const savedNetworksCacheTTL = 30 * time.Second

type Config struct {
	resource.Named
	mu                    sync.Mutex
	logger                logging.Logger
	cancelCtx             context.Context
	cancelFunc            func()
	wifiMonitor           WifiMonitor
	networkManager        WifiNetworkManager
	savedNetworksCache    []string
	savedNetworksCacheExp time.Time
}

func init() {
	resource.RegisterComponent(
		API,
		Model,
		resource.Registration[sensor.Sensor, *ComponentConfig]{Constructor: NewSensor})
}

func NewSensor(ctx context.Context, deps resource.Dependencies, conf resource.Config, logger logging.Logger) (sensor.Sensor, error) {
	logger.Infof("Starting %s %s", PrettyName, Version)
	cancelCtx, cancelFunc := context.WithCancel(context.Background())

	b := Config{
		Named:      conf.ResourceName().AsNamed(),
		logger:     logger,
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		mu:         sync.Mutex{},
	}

	if err := b.Reconfigure(ctx, deps, conf); err != nil {
		return nil, err
	}
	return &b, nil
}

func (c *Config) Reconfigure(ctx context.Context, _ resource.Dependencies, conf resource.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logger.Debugf("Reconfiguring %s", PrettyName)

	// There is no conf for this sensor
	newConf, err := resource.NativeConfig[*ComponentConfig](conf)
	if err != nil {
		return err
	}

	// In case the module has changed name
	c.Named = conf.ResourceName().AsNamed()

	mon := c.newWifiMonitor(newConf.Adapter)
	if mon == nil {
		return errors.New("no suitable wifi monitor found")
	}
	c.wifiMonitor = mon
	c.networkManager = newNetworkManager(c.logger)
	if c.networkManager == nil {
		c.logger.Warnf("nmcli not available; saved network management disabled")
	}

	return nil
}

func (c *Config) Readings(ctx context.Context, extra map[string]interface{}) (map[string]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make(map[string]interface{})
	if c.wifiMonitor != nil {
		status, err := c.wifiMonitor.GetNetworkStatus()
		if err == ErrAdapterNotFound {
			ret["err"] = "adapter not found"
		} else if err == ErrNotConnected {
			ret["err"] = "not connected to a network"
		} else if err != nil {
			c.logger.Infof("Error getting network status: %v", err)
			return nil, err
		} else {
			ret["network"] = status.NetworkName
			ret["signal_strength"] = status.SignalStrength
			ret["tx_speed_mbps"] = status.TxSpeedMbps
			ret["rx_speed_mbps"] = status.RxSpeedMbps
			ret["frequency_mhz"] = status.FrequencyMHz
			ret["tx_retries"] = status.TxRetries
			ret["tx_failed"] = status.TxFailed
			ret["beacon_signal_avg"] = status.BeaconSignalAvg
			ret["signal_avg"] = status.SignalAvg
			ret["ack_signal_avg"] = status.AckSignalAvg
			ret["noise"] = status.Noise
			ret["connected_time_sec"] = status.ConnectedTimeSec
			ret["inactive_time_ms"] = status.InactiveTimeMs
		}
	} else {
		ret["network"] = "unknown"
	}

	if c.networkManager != nil {
		networks, err := c.getSavedNetworks()
		if err != nil {
			c.logger.Warnf("Failed to list saved networks: %v", err)
		} else {
			ret["saved_networks"] = stringsToInterfaces(networks)
		}
	} else {
		ret["saved_networks_unavailable"] = true
	}

	return ret, nil
}

// getSavedNetworks returns cached saved networks, refreshing if expired.
// Must be called with c.mu held.
func (c *Config) getSavedNetworks() ([]string, error) {
	if time.Now().Before(c.savedNetworksCacheExp) {
		return c.savedNetworksCache, nil
	}
	networks, err := c.networkManager.ListSavedNetworks()
	if err != nil {
		return nil, err
	}
	c.savedNetworksCache = networks
	c.savedNetworksCacheExp = time.Now().Add(savedNetworksCacheTTL)
	return networks, nil
}

func (c *Config) invalidateSavedNetworksCache() {
	c.savedNetworksCacheExp = time.Time{}
}

func (c *Config) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	command, ok := cmd["command"].(string)
	if !ok {
		return nil, errors.New("missing or invalid 'command' field")
	}

	switch command {
	case "list_saved_networks":
		return c.handleListNetworks()
	case "forget_network":
		return c.handleForgetNetwork(cmd)
	default:
		return nil, fmt.Errorf("unknown command: %s", command)
	}
}

func (c *Config) handleListNetworks() (map[string]interface{}, error) {
	if c.networkManager == nil {
		return nil, ErrNmcliNotAvailable
	}
	networks, err := c.getSavedNetworks()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"networks": stringsToInterfaces(networks)}, nil
}

func (c *Config) handleForgetNetwork(cmd map[string]interface{}) (map[string]interface{}, error) {
	if c.networkManager == nil {
		return nil, ErrNmcliNotAvailable
	}
	name, ok := cmd["name"].(string)
	if !ok {
		return nil, errors.New("missing or invalid 'name' parameter for forget_network command")
	}
	if name == "" {
		return nil, errors.New("network name cannot be empty")
	}

	if err := c.networkManager.ForgetNetwork(name); err != nil {
		return nil, err
	}
	c.invalidateSavedNetworksCache()

	result := map[string]interface{}{"status": "ok", "name": name}
	if c.wifiMonitor != nil {
		status, err := c.wifiMonitor.GetNetworkStatus()
		if err == nil && status.NetworkName == name {
			result["warning"] = "forgot the active network; device may lose connectivity. If viam-agent provisioning is enabled, it will start the hotspot flow."
		}
	}
	return result, nil
}

func (c *Config) Close(ctx context.Context) error {
	c.logger.Infof("Shutting down %s", PrettyName)
	c.cancelFunc()
	return nil
}

func (c *Config) Ready(ctx context.Context, extra map[string]interface{}) (bool, error) {
	return false, nil
}

func stringsToInterfaces(s []string) []interface{} {
	r := make([]interface{}, len(s))
	for i, v := range s {
		r[i] = v
	}
	return r
}
