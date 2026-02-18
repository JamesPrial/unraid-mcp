package system

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FileSystemMonitor implements SystemMonitor by reading data from the local
// filesystem paths used by a running Unraid server.
type FileSystemMonitor struct {
	procPath   string
	sysPath    string
	emhttpPath string
}

// NewFileSystemMonitor returns a new FileSystemMonitor configured to read from
// the given directory paths.
//
//   - procPath    is the path to the proc filesystem directory (normally /proc).
//   - sysPath     is the path to the sys filesystem directory (normally /sys).
//   - emhttpPath  is the path to the emhttp state directory (normally /var/local/emhttp).
func NewFileSystemMonitor(procPath, sysPath, emhttpPath string) *FileSystemMonitor {
	return &FileSystemMonitor{
		procPath:   procPath,
		sysPath:    sysPath,
		emhttpPath: emhttpPath,
	}
}

// GetOverview reads CPU usage from {procPath}/stat, memory from {procPath}/meminfo,
// and temperatures from {sysPath}/hwmon/*/temp*_input.
func (m *FileSystemMonitor) GetOverview(ctx context.Context) (*SystemOverview, error) {
	ov := &SystemOverview{}

	// --- CPU ---
	cpu, err := m.parseCPUStat()
	if err != nil {
		return nil, fmt.Errorf("read cpu stat: %w", err)
	}
	ov.CPUUsagePercent = cpu

	// --- Memory ---
	if err := m.parseMemInfo(ov); err != nil {
		return nil, fmt.Errorf("read meminfo: %w", err)
	}

	// --- Temperatures ---
	temps, err := m.readTemperatures()
	if err != nil {
		// Non-fatal: missing hwmon just means no sensor data.
		temps = nil
	}
	ov.Temperatures = temps

	return ov, nil
}

// parseCPUStat reads the aggregate cpu line from {procPath}/stat and returns
// the usage percentage.
//
// Format:  cpu  user nice system idle iowait irq softirq steal guest guest_nice
// Usage  = (total - idle) / total * 100
func (m *FileSystemMonitor) parseCPUStat() (float64, error) {
	path := filepath.Join(m.procPath, "stat")
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		// Parse all numeric fields after the "cpu" label.
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, fmt.Errorf("unexpected cpu line format: %q", line)
		}

		var total, idle float64
		for i, field := range fields[1:] {
			val, err := strconv.ParseFloat(field, 64)
			if err != nil {
				return 0, fmt.Errorf("parse cpu field %d: %w", i, err)
			}
			total += val
			// Field index 3 (fields[1:][3]) is idle; index 4 is iowait.
			if i == 3 {
				idle = val
			}
		}

		if total == 0 {
			return 0, nil
		}
		return (total - idle) / total * 100, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan %s: %w", path, err)
	}
	return 0, fmt.Errorf("no aggregate cpu line found in %s", path)
}

// parseMemInfo reads {procPath}/meminfo and populates the memory fields of ov.
func (m *FileSystemMonitor) parseMemInfo(ov *SystemOverview) error {
	path := filepath.Join(m.procPath, "meminfo")
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Each line looks like:  MemTotal:       32768000 kB
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		// Remove trailing " kB" if present.
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)

		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "MemTotal":
			ov.MemTotalKB = val
		case "MemFree":
			ov.MemFreeKB = val
		case "MemAvailable":
			ov.MemAvailableKB = val
		case "SwapTotal":
			ov.SwapTotalKB = val
		case "SwapFree":
			ov.SwapFreeKB = val
		}
	}
	return scanner.Err()
}

// readTemperatures globs {sysPath}/hwmon/*/temp*_input and converts each
// millidegree value to degrees Celsius.
func (m *FileSystemMonitor) readTemperatures() ([]Temperature, error) {
	pattern := filepath.Join(m.sysPath, "hwmon", "*", "temp*_input")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}

	var temps []Temperature
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(data))
		millideg, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}

		// Build a short label from the last two path components: hwmonN/tempX_input.
		rel, err := filepath.Rel(m.sysPath, path)
		if err != nil {
			rel = path
		}
		// Strip the "_input" suffix from the label for readability.
		label := strings.TrimSuffix(rel, "_input")

		temps = append(temps, Temperature{
			Label:   label,
			Celsius: millideg / 1000.0,
		})
	}
	return temps, nil
}

// GetArrayStatus reads {emhttpPath}/var.ini and returns the current array state.
func (m *FileSystemMonitor) GetArrayStatus(ctx context.Context) (*ArrayStatus, error) {
	path := filepath.Join(m.emhttpPath, "var.ini")
	kv, err := parseKeyValueIni(path)
	if err != nil {
		return nil, fmt.Errorf("read var.ini: %w", err)
	}

	as := &ArrayStatus{}
	as.State = stripQuotes(kv["mdState"])
	as.NumDisks = parseInt(kv["mdNumDisks"])
	as.NumProtected = parseInt(kv["mdNumProtected"])
	as.NumInvalid = parseInt(kv["mdNumInvalid"])
	as.SyncErrors = parseInt(kv["sbSyncErrs"])

	resync := parseInt(kv["mdResync"])
	if resync != 0 {
		pos := parseFloat(kv["mdResyncPos"])
		size := parseFloat(kv["mdResyncSize"])
		if size > 0 {
			as.SyncProgress = pos / size * 100
		}
	}

	return as, nil
}

// GetDiskInfo reads {emhttpPath}/disks.ini and returns per-disk information.
// The file uses a PHP-style ini format with [section] headers.
func (m *FileSystemMonitor) GetDiskInfo(ctx context.Context) ([]DiskInfo, error) {
	path := filepath.Join(m.emhttpPath, "disks.ini")
	sections, err := parseSectionedIni(path)
	if err != nil {
		return nil, fmt.Errorf("read disks.ini: %w", err)
	}

	disks := make([]DiskInfo, 0, len(sections))
	for _, section := range sections {
		kv := section.kv
		d := DiskInfo{
			Name:   stripQuotes(kv["name"]),
			Device: stripQuotes(kv["device"]),
			Temp:   parseInt(kv["temp"]),
			Status: stripQuotes(kv["status"]),
			FsType: stripQuotes(kv["fsType"]),
			FsSize: parseUint(kv["fsSize"]),
			FsUsed: parseUint(kv["fsUsed"]),
		}
		// Fall back to section header name if the name key is missing.
		if d.Name == "" {
			d.Name = section.name
		}
		disks = append(disks, d)
	}
	return disks, nil
}

// ---------------------------------------------------------------------------
// Internal ini parsers
// ---------------------------------------------------------------------------

// keyValueIni is a flat map of key → raw value (with quotes).
type keyValueIni = map[string]string

// parseKeyValueIni reads a flat key="value" file (like var.ini) with no
// section headers. Blank lines and lines not containing "=" are ignored.
func parseKeyValueIni(path string) (keyValueIni, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		result[key] = val
	}
	return result, scanner.Err()
}

// iniSection holds the name of a section and its key-value pairs.
type iniSection struct {
	name string
	kv   map[string]string
}

// parseSectionedIni reads a [section]-style ini file (like disks.ini) and
// returns sections in the order they appear in the file.
func parseSectionedIni(path string) ([]iniSection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var sections []iniSection
	currentIdx := -1

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := line[1 : len(line)-1]
			sections = append(sections, iniSection{name: name, kv: make(map[string]string)})
			currentIdx = len(sections) - 1
			continue
		}
		if currentIdx < 0 {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		sections[currentIdx].kv[key] = val
	}
	return sections, scanner.Err()
}

// ---------------------------------------------------------------------------
// Value conversion helpers
// ---------------------------------------------------------------------------

// stripQuotes removes surrounding double-quotes from a raw ini value.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// parseInt parses a quoted or unquoted integer value; returns 0 on error.
func parseInt(s string) int {
	v, err := strconv.Atoi(stripQuotes(s))
	if err != nil {
		return 0
	}
	return v
}

// parseUint parses a quoted or unquoted unsigned integer value; returns 0 on error.
func parseUint(s string) uint64 {
	v, err := strconv.ParseUint(stripQuotes(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseFloat parses a quoted or unquoted float value; returns 0 on error.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(stripQuotes(s), 64)
	if err != nil {
		return 0
	}
	return v
}
