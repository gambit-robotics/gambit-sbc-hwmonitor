package sensors

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculateUsage(t *testing.T) {
	tests := []struct {
		name     string
		prev     CPUCoreStats
		curr     CPUCoreStats
		expected float64
	}{
		{
			name:     "normal usage ~50%",
			prev:     CPUCoreStats{User: 100, System: 100, Idle: 200},
			curr:     CPUCoreStats{User: 150, System: 150, Idle: 300},
			expected: 50.0,
		},
		{
			name:     "100% usage",
			prev:     CPUCoreStats{User: 100, Idle: 100},
			curr:     CPUCoreStats{User: 200, Idle: 100},
			expected: 100.0,
		},
		{
			name:     "0% usage (all idle)",
			prev:     CPUCoreStats{User: 100, Idle: 100},
			curr:     CPUCoreStats{User: 100, Idle: 200},
			expected: 0.0,
		},
		{
			name:     "counter regression - total went backwards",
			prev:     CPUCoreStats{User: 200, Idle: 200},
			curr:     CPUCoreStats{User: 100, Idle: 100},
			expected: -1, // Invalid reading
		},
		{
			name:     "counter regression - idle went backwards",
			prev:     CPUCoreStats{User: 100, Idle: 200},
			curr:     CPUCoreStats{User: 200, Idle: 100},
			expected: -1, // Invalid reading
		},
		{
			name:     "no change",
			prev:     CPUCoreStats{User: 100, Idle: 100},
			curr:     CPUCoreStats{User: 100, Idle: 100},
			expected: -1, // No delta means invalid
		},
		{
			name:     "fractional values - preserves precision",
			prev:     CPUCoreStats{User: 100.5, System: 50.25, Idle: 200.75},
			curr:     CPUCoreStats{User: 101.0, System: 50.75, Idle: 201.25},
			expected: 66.67, // totalDelta=1.5, idleDelta=0.5, (1.5-0.5)/1.5*100 = 66.67%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateUsage(tt.prev, tt.curr)
			if result != tt.expected {
				t.Errorf("CalculateUsage() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGetProcCmdlineParseing(t *testing.T) {
	data := "/root/.viam/packages-local/data/module/synthetic-rinzlerlabs_sbc-hwmonitor_from_reload-0_0_21/bin/rinzlerlabs-sbc-hwmonitor\x00/tmp/viam-module-2615955240/rinzlerlabs_sbc-hwmonitor_from_reload-sJYoJ.sock\x00"
	// The cmdline is null-separated, so split it and return the first argument
	args := strings.Split(string(data), "\x00")
	if len(args) > 0 {

		fmt.Printf("First argument in cmdline: %s\n", filepath.Base(args[0]))
		return
	}
	t.FailNow()
}
