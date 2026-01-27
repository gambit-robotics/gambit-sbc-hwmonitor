package sensors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rinzlerlabs/viam-sbc-hwmonitor/utils"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"
	"go.viam.com/rdk/logging"
)

var (
	ErrProcessNotFound = errors.New("process not found")
)

type CPUCoreStats struct {
	User    float64
	Nice    float64
	System  float64
	Idle    float64
	IOWait  float64
	IRQ     float64
	SoftIRQ float64
	Steal   float64
}

type Process struct {
	*process.Process
	PID     int32
	Name    string // Name of the process (for convenience)
	exe     string // Path to the executable
	cmdline string // Command line of the process
}

func (p *Process) Exe() (string, error) {
	if p.exe != "" {
		return p.exe, nil
	}
	if p.Process == nil {
		return "", errors.New("process is nil")
	}
	exe, err := p.Process.Exe()
	if err != nil {
		return "", err
	}
	p.exe = exe // Cache the executable path
	return exe, nil
}

func (p *Process) Cmdline() (string, error) {
	if p.cmdline != "" {
		return p.cmdline, nil
	}
	if p.Process == nil {
		return "", errors.New("process is nil")
	}
	cmdline, err := p.Process.Cmdline()
	if err != nil {
		return "", err
	}
	p.cmdline = cmdline // Cache the command line
	return cmdline, nil
}

func ReadCPUStats() (map[string]CPUCoreStats, error) {
	rawStats, err := cpu.Times(true)
	if err != nil {
		return nil, err
	}
	stats := make(map[string]CPUCoreStats)
	totalStats := CPUCoreStats{}
	for _, stat := range rawStats {
		// Add per-core stats (keep float64 precision from gopsutil)
		stats[stat.CPU] = CPUCoreStats{
			User:    stat.User,
			Nice:    stat.Nice,
			System:  stat.System,
			Idle:    stat.Idle,
			IOWait:  stat.Iowait,
			IRQ:     stat.Irq,
			SoftIRQ: stat.Softirq,
			Steal:   stat.Steal,
		}

		// Add total stats
		totalStats.User += stat.User
		totalStats.Nice += stat.Nice
		totalStats.System += stat.System
		totalStats.Idle += stat.Idle
		totalStats.IOWait += stat.Iowait
		totalStats.IRQ += stat.Irq
		totalStats.SoftIRQ += stat.Softirq
		totalStats.Steal += stat.Steal
	}

	stats["cpu"] = totalStats

	return stats, nil
}

type ProcessMonitor struct {
	Processes         utils.OrderedMap[int32, *Process] // List of processes to monitor
	lastSync          time.Time
	name              string
	disablePidCaching bool
	logger            logging.Logger
	mu                sync.Mutex
}

// CalculateUsage calculates CPU usage percentages.
// Returns -1 if counter regression is detected (caller should skip this reading).
// Counter regression can occur due to:
// - Kernel race conditions (no mutex on CPU metric counters)
// - CPU hotplug events (common on ARM SoCs with power management)
// - System suspension
// See: https://github.com/prometheus/node_exporter/pull/2067
func CalculateUsage(prev, curr CPUCoreStats) float64 {
	prevIdle := prev.Idle + prev.IOWait
	currIdle := curr.Idle + curr.IOWait

	prevNonIdle := prev.User + prev.Nice + prev.System + prev.IRQ + prev.SoftIRQ + prev.Steal
	currNonIdle := curr.User + curr.Nice + curr.System + curr.IRQ + curr.SoftIRQ + curr.Steal

	prevTotal := prevIdle + prevNonIdle
	currTotal := currIdle + currNonIdle

	// Guard against counter regression (values going backwards)
	// Return -1 to signal invalid reading - caller should keep previous value
	if currTotal <= prevTotal {
		return -1
	}
	if currIdle < prevIdle {
		return -1
	}

	totalDelta := currTotal - prevTotal
	idleDelta := currIdle - prevIdle

	usage := ((totalDelta - idleDelta) / totalDelta) * 100

	// Clamp to valid range
	if usage < 0 {
		return 0.0
	}
	if usage > 100 {
		return 100.0
	}

	return utils.RoundValue(usage, 2)
}

func NewProcessMonitor(logger logging.Logger, name string, disablePidCaching bool) *ProcessMonitor {
	// Initialize a new process monitor
	pm := &ProcessMonitor{
		Processes:         utils.NewOrderedMap[int32, *Process](),
		name:              name,
		disablePidCaching: disablePidCaching,
		logger:            logger,
	}
	return pm
}

func (p *ProcessMonitor) GetProcessesWithContext(ctx context.Context) (utils.OrderedMap[int32, *Process], error) {
	p.mu.Lock()
	defer p.mu.Unlock() // Ensure the mutex is unlocked after the function completes
	if p.Processes.Len() > 0 {
		if !p.disablePidCaching {
			if p.lastSync.Add(10 * time.Second).Before(time.Now()) {
				p.logger.Debugf("Returning %d cached processes, last sync: %v, current time: %v", p.Processes.Len(), p.lastSync, time.Now())
				return p.Processes, nil // Return cached processes if within the sync interval
			} else {
				// If within the sync interval and pid caching is enabled, return cached processes
				p.logger.Debugf("Have %d cached processes, but sync interval elapsed: %v", p.Processes.Len(), p.lastSync.Add(10*time.Second))
				return p.Processes, nil
			}
		} else {
			// If pid caching is disabled, always perform a sync
			p.logger.Debugf("Have %d cached processes, but pid caching is disabled, performing a sync", p.Processes.Len())
		}
	} else {
		p.logger.Debugf("No cached processes found for %s, performing a sync", p.name)
	}

	p.logger.Debugf("Syncing processes for %s, last sync: %v, current time: %v, disablePidCaching: %v", p.name, p.lastSync, time.Now(), p.disablePidCaching)

	for _, pid := range slices.Collect(p.Processes.Keys()) {
		if ret, err := process.PidExistsWithContext(ctx, int32(pid)); err != nil || !ret {
			// Remove the process from the map if it no longer exists
			p.Processes.Delete(pid) // Keep the process in the list if it still exists
		}
	}

	process.EnableBootTimeCache(true)
	ret := utils.NewOrderedMap[int32, *Process]()
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		p.logger.Debugf("Failed to get processes: %v", err)
		return nil, errors.Join(errors.New("failed to get processes"), err)
	}
	for _, proc := range procs {
		// The linux kernel seems to limit the contents of /proc/<pid>/comm to 15 bytes,
		// if the process name is longer than that we need to fall back to /proc/<pid>/cmdline
		if len(p.name) <= 15 {
			procName, err := getProcName(proc) // Get the process name
			if err != nil {
				p.logger.Debugf("Failed to get process name for PID %d: %v", proc.Pid, err)
				continue
			}
			if procName == p.name {
				p.logger.Debugf("Found process %s with PID %d", procName, proc.Pid)
				ret.Set(proc.Pid, &Process{Process: proc, PID: proc.Pid, Name: procName}) // Store the process in the ordered map
				continue
			}
		} else {
			cmdline, err := getProcCmdline(proc)
			if err != nil {
				p.logger.Debugf("Failed to get process cmdline for PID %d: %v", proc.Pid, err)
				continue
			}
			if cmdline == "" {
				continue
			}
			if filepath.Base(cmdline) == p.name {
				p.logger.Debugf("Found process %s with PID %d", filepath.Base(cmdline), proc.Pid)
				ret.Set(proc.Pid, &Process{Process: proc, PID: proc.Pid, Name: filepath.Base(cmdline)}) // Store the process in the ordered map
				continue
			}
			if cmdline == p.name {
				p.logger.Debugf("Found process %s with PID %d", cmdline, proc.Pid)
				ret.Set(proc.Pid, &Process{Process: proc, PID: proc.Pid, Name: cmdline}) // Store the process in the ordered map
				continue
			}
		}
	}
	p.Processes = ret
	p.lastSync = time.Now() // Update the last sync time
	p.logger.Debugf("Synced processes for %s, found %d processes", p.name, ret.Len())
	return ret, nil
}

func getProcName(proc *process.Process) (string, error) {
	pid := proc.Pid
	statPath := filepath.Join("/proc", strconv.Itoa(int(pid)), "comm")
	contents, err := os.ReadFile(statPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(contents), "\n"), nil
}

func getProcCmdline(proc *process.Process) (string, error) {
	cmdlinePath := filepath.Join("/proc", strconv.Itoa(int(proc.Pid)), "cmdline")
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return "", err
	}
	// The cmdline is null-separated, so split it and return the first argument
	args := strings.Split(string(data), "\x00")
	if len(args) > 0 {
		return args[0], nil
	}
	return "", errors.New("cmdline is empty")
}
