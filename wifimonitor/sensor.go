package wifimonitor

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

type Config struct {
	resource.Named
	mu             sync.Mutex
	logger         logging.Logger
	cancelCtx      context.Context
	cancelFunc     func()
	wifiMonitor    WifiMonitor
	profileManager networkProfileManager
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

	c.profileManager = c.newNetworkProfileManager()
	if c.profileManager == nil {
		c.logger.Warn("no nmcli found on PATH; list_saved_networks / forget_network will be unavailable")
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

	return ret, nil
}

func (c *Config) Close(ctx context.Context) error {
	c.logger.Infof("Shutting down %s", PrettyName)
	c.cancelFunc()
	return nil
}

func (c *Config) Ready(ctx context.Context, extra map[string]interface{}) (bool, error) {
	return false, nil
}

// DoCommand handles wifi profile management commands.
//
// Supported commands:
//
//	{"command": "list_saved_networks"}
//	    -> {"networks": ["ssid1", "ssid2", ...]}
//
//	{"command": "forget_network", "name": "ssid1"}
//	    -> {"forgotten": true, "name": "ssid1"}
//
// Errors are always returned to the caller AND logged — no silent swallowing.
func (c *Config) DoCommand(ctx context.Context, cmd map[string]interface{}) (map[string]interface{}, error) {
	cmdName, _ := cmd["command"].(string)
	c.logger.Infof("DoCommand received: command=%q", cmdName)

	// Snapshot the manager under the mutex — matches Readings/Reconfigure
	// locking so a concurrent Reconfigure can't swap profileManager mid-call.
	// Don't hold mu across the nmcli exec; profile-management commands can
	// take seconds and we don't want to block Readings() that whole time.
	c.mu.Lock()
	mgr := c.profileManager
	c.mu.Unlock()

	res, err := dispatchCommand(mgr, cmdName, cmd)
	if err != nil {
		c.logger.Warnf("DoCommand %q failed: %v", cmdName, err)
		return nil, err
	}
	c.logger.Infof("DoCommand %q succeeded", cmdName)
	return res, nil
}

func dispatchCommand(mgr networkProfileManager, cmdName string, cmd map[string]interface{}) (map[string]interface{}, error) {
	switch cmdName {
	case "":
		return nil, errors.New("missing 'command' field")
	case "list_saved_networks":
		if mgr == nil {
			return nil, errors.New("list_saved_networks unavailable: nmcli not found on PATH")
		}
		names, err := mgr.ListSavedNetworks()
		if err != nil {
			return nil, fmt.Errorf("list_saved_networks: %w", err)
		}
		return map[string]interface{}{"networks": names}, nil
	case "forget_network":
		if mgr == nil {
			return nil, errors.New("forget_network unavailable: nmcli not found on PATH")
		}
		name, _ := cmd["name"].(string)
		if name == "" {
			return nil, errors.New("forget_network requires 'name'")
		}
		if err := validateProfileName(name); err != nil {
			return nil, fmt.Errorf("forget_network: %w", err)
		}
		if err := mgr.ForgetNetwork(name); err != nil {
			return nil, fmt.Errorf("forget_network %q: %w", name, err)
		}
		return map[string]interface{}{"forgotten": true, "name": name}, nil
	default:
		return nil, fmt.Errorf("unknown command %q", cmdName)
	}
}
