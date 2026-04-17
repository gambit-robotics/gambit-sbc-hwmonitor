package wifimonitor

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"go.viam.com/rdk/logging"
)

// networkProfileManager manages stored wifi network profiles (NetworkManager
// connections). Separate from WifiMonitor, which only reads live link stats.
type networkProfileManager interface {
	ListSavedNetworks() ([]string, error)
	ForgetNetwork(name string) error
}

type nmcliNetworkProfileManager struct {
	logger logging.Logger
}

func (m *nmcliNetworkProfileManager) ListSavedNetworks() ([]string, error) {
	// -t = terse (colon-separated, backslash-escaped), -f = fields.
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,UUID,TYPE,DEVICE", "connection", "show")
	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExecErr("nmcli connection show", err)
	}
	return parseNmcliConnectionList(string(out))
}

func (m *nmcliNetworkProfileManager) ForgetNetwork(name string) error {
	if err := validateProfileName(name); err != nil {
		return err
	}
	m.logger.Infof("forgetting wifi profile %q via nmcli connection delete", name)
	cmd := exec.Command("nmcli", "connection", "delete", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return wrapExecErr(fmt.Sprintf("nmcli connection delete %q: %s", name, strings.TrimSpace(string(out))), err)
	}
	m.logger.Infof("forgot wifi profile %q: %s", name, strings.TrimSpace(string(out)))
	return nil
}

func wrapExecErr(ctx string, err error) error {
	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("%s: %w (stderr=%s)", ctx, err, strings.TrimSpace(string(ee.Stderr)))
	}
	return fmt.Errorf("%s: %w", ctx, err)
}

// parseNmcliConnectionList parses terse output of:
//
//	nmcli -t -f NAME,UUID,TYPE,DEVICE connection show
//
// and returns the NAME of every wifi connection in the input order.
// nmcli's terse format backslash-escapes ':' and '\' inside fields.
func parseNmcliConnectionList(out string) ([]string, error) {
	const wifiType = "802-11-wireless"
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := splitTerseNmcli(line)
		if len(fields) < 3 {
			continue
		}
		if fields[2] != wifiType {
			continue
		}
		names = append(names, fields[0])
	}
	return names, nil
}

// splitTerseNmcli splits a single line of nmcli -t output on unescaped ':'.
// A literal ':' or '\' in a field appears as '\:' or '\\'.
func splitTerseNmcli(line string) []string {
	var (
		fields []string
		cur    strings.Builder
		i      int
	)
	for i < len(line) {
		c := line[i]
		if c == '\\' && i+1 < len(line) {
			cur.WriteByte(line[i+1])
			i += 2
			continue
		}
		if c == ':' {
			fields = append(fields, cur.String())
			cur.Reset()
			i++
			continue
		}
		cur.WriteByte(c)
		i++
	}
	fields = append(fields, cur.String())
	return fields
}

// validateProfileName rejects names that could be misinterpreted as flags by
// nmcli or would inject control characters. Profile names in NetworkManager
// can contain spaces, unicode, and colons, but should never contain newlines
// or null bytes, and must not start with '-'.
func validateProfileName(name string) error {
	if name == "" {
		return errors.New("profile name is empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("profile name %q starts with '-' and could be interpreted as a flag", name)
	}
	if strings.ContainsAny(name, "\n\r\x00") {
		return fmt.Errorf("profile name contains control character")
	}
	return nil
}
