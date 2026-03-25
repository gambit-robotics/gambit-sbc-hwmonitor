# gambit-robotics:sbc-hwmonitor

This is a Viam Module that contains a number of sensors and utilities for single board computers (or any linux machine).

While this package strives to use no external libraries and executables, sometimes that is unavoidable. For the Raspberry Pi, some values are derived from the [`vcgencmd`](https://github.com/raspberrypi/documentation/blob/16480247dcac12d1f828c0f2556a3bc430de3c90/raspbian/applications/vcgencmd.md).

## clocks

This sensor reports the clock frequencies of various components on the SBC. For the Raspberry Pi, this requires the `vcgencmd` to be present.

## cpu_manager

This is both a sensor and a configuration utility. It lets you manage the CPU frequency and governor of the Raspberry PI CPU. Please note, this will automatically install the `cpufrequtils` package using the package manager available on the system.

Sample Config
```json
{
  "governor": "<governor>",
  "frequency": 0,
  "minimum": 0,
  "maximum": 0
}
```

## cpu_monitor

This is a basic CPU monitor that reports per-core and overall usage percentages.

Sample Config
```json
{
  "sleep_time_ms": 1000
}
```

## disk_monitor

This sensor reports disk usage (total, used, free, percent used) for detected disk partitions. You can optionally filter to specific disks and include IO counters.

Sample Config
```json
{
  "disks": ["/dev/sda1", "/"],
  "include_io_counters": true
}
```

If `disks` is empty, all real disk partitions are reported. Disks can be matched by device path, device name, or mountpoint.

## gpu_monitor

This sensor reports GPU usage statistics. Supports NVIDIA Jetson boards (via built-in tegrastats) and any system with the `nvidia-smi` command.

## memory_monitor

This is a basic memory stats for the SBC.

## power_manager

This sensor reports and manages the CPU/power configuration of an SBC. It supports both Raspberry Pi and NVIDIA Jetson boards. This will automatically install the `cpufrequtils` package.

Sample Config (Raspberry Pi)
```json
{
  "raspi": {
    "governor": "<governor>",
    "frequency": 0,
    "minimum": 0,
    "maximum": 0
  }
}
```

Sample Config (Jetson)
```json
{
  "jetson": {
    "power_mode": 0,
    "governor": "<governor>",
    "frequency": 0,
    "minimum": 0,
    "maximum": 0
  }
}
```

## process_monitor

This lets you monitor a specific process and get more information about the environment under which it is running.

Sample Config
```json
{
  "name": "<name>",
  "executable_path": "/absolute/path/to/executable",
  "include_env": false,
  "include_cmdline": false,
  "include_cwd": false,
  "include_open_file_count": false,
  "include_mem_info": false,
  "include_open_files": false,
  "include_ulimits": false,
  "include_net_stats": false,
  "sleep_time_ms": 0,
  "disable_pid_caching": false
}
```

Either `name` or `executable_path` must be provided, but not both.

## pwm_fan

This lets you control a cooling fan for the SBC based on the CPU temperatures. For the Raspberry Pi 5, the built-in fan is supported via `use_internal_fan`. For other boards, a GPIO pin and board name must be provided.

Sample Config (Raspberry Pi 5 internal fan)
```json
{
  "use_internal_fan": true,
  "temperature_table": {
    "50": 25,
    "60": 50,
    "70": 75,
    "80": 100
  }
}
```

Sample Config (external fan via GPIO)
```json
{
  "fan_pin": "<pin>",
  "board_name": "<board>",
  "temperature_table": {
    "50": 25,
    "60": 50,
    "70": 75,
    "80": 100
  }
}
```

Temperature table keys are temperature thresholds (as strings) and values are fan speed percentages (0-100).

## temperature

This reports the temperature of various temperature sensors. Available sensors vary by board.

## throttling

This reports the throttling state of various components of the SBC.

## voltages

This reports the voltages of various components on the board. The CPU voltages are generally available for all boards. Some boards also include GPU and total system power.

## wifi_monitor

This sensor reports Wi-Fi connection status including signal strength, TX/RX speeds, frequency, retries, noise, and connection time. On Linux, it uses `iw`, `nmcli`, or `/proc/net/wireless` (whichever is available). Requires an adapter name.

Sample Config
```json
{
  "adapter": "wlan0"
}
```

## Building

The module targets `linux/arm64` and `linux/amd64`. The default Makefile build cross-compiles for `linux/arm64`:

```bash
make build          # cross-compile the binary
make test           # build then run go test ./...
make package        # build, download gopsutil LICENSE, and create tar.gz
make clean          # remove bin/, package/, and gopsutil_LICENSE
```

To build for a different architecture, override `GOARCH` on the command line:

```bash
make build GOARCH=amd64
```

## Releasing a New Version

### Prerequisites

- Go 1.23+
- The [Viam CLI](https://docs.viam.com/cli/) (`viam`) — only needed for manual uploads
- Git with push access to this repository

### Automated release (CI)

The GitHub Actions workflow (`.github/workflows/go.yml`) handles building and publishing automatically:

1. Update the version string in `utils/version.go`
2. Commit and push the change
3. Tag the commit and push the tag:
   ```bash
   git tag v<version>
   git push && git push --tags
   ```

CI will run tests, then the `publish` job uses `viamrobotics/build-action` to build for each platform in `meta.json` (`linux/arm64`, `linux/amd64`) and upload the module to Viam.

### Manual release

If you need to upload manually (requires Viam CLI authentication):

```bash
make upload
```

The `upload` target cross-compiles for `linux/arm64`, packages the binary with `meta.json` and the gopsutil LICENSE into a tar.gz, runs `viam module update`, and then `viam module upload`. It will fail if:
- The version in `utils/version.go` doesn't match the latest git tag
- HEAD isn't tagged
- There are uncommitted changes

### Verifying a release

After CI completes (or after a manual upload), check that the new version appears in the [Viam registry](https://app.viam.com/registry) under the `gambit-robotics:sbc-hwmonitor` module.
