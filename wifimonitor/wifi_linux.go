package wifimonitor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"go.viam.com/rdk/logging"
)

func (c *Config) newWifiMonitor(adapter string) WifiMonitor {
	// iw has the best stats
	if _, err := exec.LookPath("iw"); err == nil {
		c.logger.Infof("Using iw for wifi stats")
		return &iwWifiMonitor{adapter: adapter, logger: c.logger}
	}
	// nmcli has good stats
	if _, err := exec.LookPath("nmcli"); err == nil {
		c.logger.Infof("Using nmcli for wifi stats")
		return &nmcliWifiMonitor{adapter: adapter, logger: c.logger}
	}
	// proc has basic stats
	if _, err := os.Stat("/proc/net/wireless"); err == nil {
		c.logger.Infof("Using /proc/net/wireless for wifi stats")
		return &procWifiMonitor{adapter: adapter, logger: c.logger}
	}
	return nil
}

type nmcliWifiMonitor struct {
	logger  logging.Logger
	adapter string
}

func (w *nmcliWifiMonitor) GetNetworkStatus() (*networkStatus, error) {
	cmd := exec.Command("nmcli", "-t", "-f", "ACTIVE,NAME,SSID,CHAN,FREQ,RATE,SIGNAL,DEVICE", "dev", "wifi")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return w.parseNetworkStatus(string(out))
}

func (w *nmcliWifiMonitor) parseNetworkStatus(out string) (*networkStatus, error) {
	adapterFound := false
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if !strings.HasSuffix(line, w.adapter) {
			continue
		}
		adapterFound = true
		if strings.HasPrefix(line, "yes:") {
			var e error = nil
			col := strings.Split(line, ":")
			signalStrength, err := strconv.Atoi(col[6])
			if err != nil {
				signalStrength = -1
				e = errors.Join(e, err)
			}

			linkSpeed, err := strconv.ParseFloat(strings.Split(col[5], " ")[0], 64)
			if err != nil {
				linkSpeed = -1
				e = errors.Join(e, err)
			}

			return &networkStatus{
				NetworkName:    col[2],
				SignalStrength: -1 * signalStrength,
				TxSpeedMbps:    linkSpeed,
			}, e
		}
	}
	if !adapterFound {
		return nil, ErrAdapterNotFound
	} else {
		return nil, ErrNotConnected
	}
}

type iwWifiMonitor struct {
	logger  logging.Logger
	adapter string
}

func (w *iwWifiMonitor) GetNetworkStatus() (*networkStatus, error) {
	cmd := exec.Command("iw", "dev", w.adapter, "link")
	out, err := cmd.Output()
	if err != nil {
		if err.Error() == "exit status 237" {
			return nil, ErrAdapterNotFound
		}
		return nil, err
	}

	status, err := w.parseNetworkStatus(string(out))
	if err != nil {
		return nil, err
	}

	// Get additional stats from station dump (retries, failures, etc.)
	w.enrichWithStationDump(status)

	// Get noise floor from survey dump
	w.enrichWithSurveyDump(status)

	return status, nil
}

func (w *iwWifiMonitor) parseNetworkStatus(out string) (*networkStatus, error) {
	var e error = nil
	if strings.Contains(string(out), "Not connected") {
		return nil, ErrNotConnected
	}
	if strings.Contains(string(out), "No such device") {
		return nil, ErrAdapterNotFound
	}
	status := &networkStatus{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SSID:") {
			col := strings.Split(line, ":")
			status.NetworkName = strings.TrimSpace(col[1])
		} else if strings.HasPrefix(line, "freq:") {
			col := strings.Split(line, ":")
			freqStr := strings.TrimSpace(col[1])
			// Handle both "2412" and "5200.0" formats
			freq, err := strconv.ParseFloat(freqStr, 64)
			if err != nil {
				e = errors.Join(e, err)
			} else {
				status.FrequencyMHz = int(freq)
			}
		} else if strings.HasPrefix(line, "signal:") {
			col := strings.Split(line, ":")
			signalStrength, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(col[1]), " dBm"))
			if err != nil {
				signalStrength = -1
				e = errors.Join(e, err)
			}
			status.SignalStrength = signalStrength
		} else if strings.HasPrefix(line, "rx bitrate:") {
			col := strings.Split(line, ":")
			linkSpeed, err := strconv.ParseFloat(strings.Split(col[1], " ")[1], 64)
			if err != nil {
				linkSpeed = -1
				e = errors.Join(e, err)
			}
			status.RxSpeedMbps = linkSpeed
		} else if strings.HasPrefix(line, "tx bitrate:") {
			col := strings.Split(line, ":")
			linkSpeed, err := strconv.ParseFloat(strings.Split(col[1], " ")[1], 64)
			if err != nil {
				linkSpeed = -1
				e = errors.Join(e, err)
			}
			status.TxSpeedMbps = linkSpeed
		}
	}

	return status, e
}

// enrichWithStationDump adds retry/failure stats from iw station dump
func (w *iwWifiMonitor) enrichWithStationDump(status *networkStatus) {
	cmd := exec.Command("iw", "dev", w.adapter, "station", "dump")
	out, err := cmd.Output()
	if err != nil {
		return // silently fail - these are optional stats
	}
	w.parseStationDump(string(out), status)
}

// parseStationDump parses the output of iw station dump
func (w *iwWifiMonitor) parseStationDump(out string, status *networkStatus) {
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "tx retries:") {
			col := strings.Split(line, ":")
			if val, err := strconv.Atoi(strings.TrimSpace(col[1])); err == nil {
				status.TxRetries = val
			}
		} else if strings.HasPrefix(line, "tx failed:") {
			col := strings.Split(line, ":")
			if val, err := strconv.Atoi(strings.TrimSpace(col[1])); err == nil {
				status.TxFailed = val
			}
		} else if strings.HasPrefix(line, "beacon signal avg:") {
			col := strings.Split(line, ":")
			valStr := strings.TrimSuffix(strings.TrimSpace(col[1]), " dBm")
			if val, err := strconv.Atoi(valStr); err == nil {
				status.BeaconSignalAvg = val
			}
		} else if strings.HasPrefix(line, "signal avg:") {
			col := strings.Split(line, ":")
			valStr := strings.TrimSpace(col[1])
			// Handle format like "-49 [-57, -56, -54] dBm" by taking first number
			valStr = strings.Split(valStr, " ")[0]
			if val, err := strconv.Atoi(valStr); err == nil {
				status.SignalAvg = val
			}
		} else if strings.HasPrefix(line, "ack signal avg:") {
			col := strings.Split(line, ":")
			valStr := strings.TrimSuffix(strings.TrimSpace(col[1]), " dBm")
			if val, err := strconv.Atoi(valStr); err == nil {
				status.AckSignalAvg = val
			}
		} else if strings.HasPrefix(line, "connected time:") {
			col := strings.Split(line, ":")
			valStr := strings.TrimSuffix(strings.TrimSpace(col[1]), " seconds")
			if val, err := strconv.Atoi(valStr); err == nil {
				status.ConnectedTimeSec = val
			}
		} else if strings.HasPrefix(line, "inactive time:") {
			col := strings.Split(line, ":")
			valStr := strings.TrimSuffix(strings.TrimSpace(col[1]), " ms")
			if val, err := strconv.Atoi(valStr); err == nil {
				status.InactiveTimeMs = val
			}
		}
	}
}

// enrichWithSurveyDump adds noise floor from iw survey dump
func (w *iwWifiMonitor) enrichWithSurveyDump(status *networkStatus) {
	cmd := exec.Command("iw", "dev", w.adapter, "survey", "dump")
	out, err := cmd.Output()
	if err != nil {
		return // silently fail - this is optional
	}
	w.parseSurveyDump(string(out), status)
}

// parseSurveyDump parses the output of iw survey dump to get noise floor
// It finds the survey block matching the current frequency and extracts noise
func (w *iwWifiMonitor) parseSurveyDump(out string, status *networkStatus) {
	lines := strings.Split(out, "\n")
	inCurrentFreqBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for frequency line to identify the right block
		if strings.HasPrefix(line, "frequency:") {
			col := strings.Split(line, ":")
			freqStr := strings.TrimSpace(col[1])
			freqStr = strings.TrimSuffix(freqStr, " MHz")
			// Check for "[in use]" marker which indicates current channel
			if strings.Contains(freqStr, "[in use]") {
				inCurrentFreqBlock = true
				freqStr = strings.TrimSpace(strings.Split(freqStr, "[")[0])
			} else if status.FrequencyMHz > 0 {
				// Match by frequency if we know it
				if freq, err := strconv.Atoi(freqStr); err == nil && freq == status.FrequencyMHz {
					inCurrentFreqBlock = true
				} else {
					inCurrentFreqBlock = false
				}
			} else {
				inCurrentFreqBlock = false
			}
		} else if inCurrentFreqBlock && strings.HasPrefix(line, "noise:") {
			col := strings.Split(line, ":")
			valStr := strings.TrimSuffix(strings.TrimSpace(col[1]), " dBm")
			if val, err := strconv.Atoi(valStr); err == nil {
				status.Noise = val
			}
			return // Found what we need
		}
	}
}

type procWifiMonitor struct {
	logger  logging.Logger
	adapter string
}

func (w *procWifiMonitor) GetNetworkStatus() (*networkStatus, error) {
	out, err := os.ReadFile("/proc/net/wireless")
	if err != nil {
		return nil, err
	}
	return w.parseNetworkStatus(string(out))
}

func (w *procWifiMonitor) parseNetworkStatus(out string) (*networkStatus, error) {
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, w.adapter) {
			col := strings.Fields(line)
			signalStrength, err := strconv.Atoi(strings.TrimSuffix(col[3], "."))
			if err != nil {
				return nil, err
			}
			linkSpeed, err := strconv.ParseFloat(col[2], 64)
			if err != nil {
				return nil, err
			}
			return &networkStatus{
				NetworkName:    "unknown",
				SignalStrength: signalStrength,
				TxSpeedMbps:    linkSpeed,
			}, nil
		}
	}
	return nil, ErrAdapterNotFound
}

type nmcliNetworkManager struct {
	logger logging.Logger
}

func newNetworkManager(logger logging.Logger) WifiNetworkManager {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return nil
	}
	return &nmcliNetworkManager{logger: logger}
}

func (m *nmcliNetworkManager) ListSavedNetworks() ([]string, error) {
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	return m.parseConnectionList(string(out)), nil
}

func (m *nmcliNetworkManager) parseConnectionList(out string) []string {
	var networks []string
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if name, ok := strings.CutSuffix(line, ":802-11-wireless"); ok && name != "" {
			// nmcli -t mode escapes colons as \: in field values
			name = strings.ReplaceAll(name, "\\:", ":")
			networks = append(networks, name)
		}
	}
	return networks
}

func (m *nmcliNetworkManager) ForgetNetwork(name string) error {
	cmd := exec.Command("nmcli", "connection", "delete", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete network %q: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
