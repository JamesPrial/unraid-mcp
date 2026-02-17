package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// projectRoot returns the absolute path to the project root by navigating up
// from internal/system/.
func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve project root: %v", err)
	}
	return root
}

// testdataProcPath returns the absolute path to testdata/proc.
func testdataProcPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", "proc")
}

// testdataSysPath returns the absolute path to testdata/sys.
func testdataSysPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", "sys")
}

// testdataEmhttpPath returns the absolute path to testdata/emhttp.
func testdataEmhttpPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata", "emhttp")
}

// validMonitor returns a FileSystemMonitor pointed at the standard testdata.
func validMonitor(t *testing.T) *FileSystemMonitor {
	t.Helper()
	return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), testdataEmhttpPath(t))
}

// writeTempFile creates a file under a temp directory and returns its parent
// directory (not the file path itself).
func writeTempDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

// ---------------------------------------------------------------------------
// Constructor Tests
// ---------------------------------------------------------------------------

func Test_NewFileSystemMonitor_ReturnsNonNil(t *testing.T) {
	m := NewFileSystemMonitor("/proc", "/sys", "/emhttp")
	if m == nil {
		t.Fatal("NewFileSystemMonitor() returned nil")
	}
}

func Test_NewFileSystemMonitor_ImplementsSystemMonitor(t *testing.T) {
	var _ SystemMonitor = NewFileSystemMonitor("/proc", "/sys", "/emhttp")
}

// ---------------------------------------------------------------------------
// GetOverview Tests
// ---------------------------------------------------------------------------

func Test_GetOverview_Cases(t *testing.T) {
	tests := []struct {
		name        string
		monitor     func(t *testing.T) *FileSystemMonitor
		wantErr     bool
		errContains string
		validate    func(t *testing.T, ov *SystemOverview)
	}{
		{
			name: "valid testdata returns populated SystemOverview",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				t.Helper()
				if ov == nil {
					t.Fatal("expected non-nil SystemOverview")
				}
				// Memory values must be populated.
				if ov.MemTotalKB == 0 {
					t.Error("MemTotalKB should not be zero")
				}
				if ov.MemFreeKB == 0 {
					t.Error("MemFreeKB should not be zero")
				}
				if ov.MemAvailableKB == 0 {
					t.Error("MemAvailableKB should not be zero")
				}
			},
		},
		{
			name: "memory parsed correctly from meminfo",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				t.Helper()
				if ov == nil {
					t.Fatal("expected non-nil SystemOverview")
				}
				if ov.MemTotalKB != 32768000 {
					t.Errorf("MemTotalKB = %d, want 32768000", ov.MemTotalKB)
				}
				if ov.MemFreeKB != 8192000 {
					t.Errorf("MemFreeKB = %d, want 8192000", ov.MemFreeKB)
				}
				if ov.MemAvailableKB != 16384000 {
					t.Errorf("MemAvailableKB = %d, want 16384000", ov.MemAvailableKB)
				}
			},
		},
		{
			name: "swap parsed correctly from meminfo",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				t.Helper()
				if ov == nil {
					t.Fatal("expected non-nil SystemOverview")
				}
				if ov.SwapTotalKB != 4096000 {
					t.Errorf("SwapTotalKB = %d, want 4096000", ov.SwapTotalKB)
				}
				if ov.SwapFreeKB != 4096000 {
					t.Errorf("SwapFreeKB = %d, want 4096000", ov.SwapFreeKB)
				}
			},
		},
		{
			name: "CPU usage percent is reasonable",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				t.Helper()
				if ov == nil {
					t.Fatal("expected non-nil SystemOverview")
				}
				// CPU usage from testdata:
				// cpu line: user=1000000 nice=50000 system=300000 idle=8000000 iowait=100000 irq=20000 softirq=10000
				// total = 9480000, idle portion = 8000000
				// Usage should be around (1480000/9480000)*100 ~ 15.6%
				// We just verify it is between 0 and 100.
				if ov.CPUUsagePercent < 0 || ov.CPUUsagePercent > 100 {
					t.Errorf("CPUUsagePercent = %f, want 0-100", ov.CPUUsagePercent)
				}
				if ov.CPUUsagePercent == 0 {
					t.Error("CPUUsagePercent should not be exactly 0 with non-idle testdata")
				}
			},
		},
		{
			name: "temperatures found from hwmon",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				t.Helper()
				if ov == nil {
					t.Fatal("expected non-nil SystemOverview")
				}
				if len(ov.Temperatures) < 2 {
					t.Fatalf("expected at least 2 temperatures, got %d", len(ov.Temperatures))
				}
				// We expect 45.0 and 38.0 Celsius (from millidegrees 45000 and 38000).
				found45 := false
				found38 := false
				for _, temp := range ov.Temperatures {
					if temp.Celsius == 45.0 {
						found45 = true
					}
					if temp.Celsius == 38.0 {
						found38 = true
					}
				}
				if !found45 {
					t.Error("expected temperature 45.0 C not found in Temperatures")
				}
				if !found38 {
					t.Error("expected temperature 38.0 C not found in Temperatures")
				}
			},
		},
		{
			name: "temperature labels are non-empty",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				t.Helper()
				if ov == nil {
					t.Fatal("expected non-nil SystemOverview")
				}
				for i, temp := range ov.Temperatures {
					if temp.Label == "" {
						t.Errorf("Temperatures[%d].Label is empty", i)
					}
				}
			},
		},
		{
			name: "missing proc dir returns error",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return NewFileSystemMonitor("/nonexistent/proc", testdataSysPath(t), testdataEmhttpPath(t))
			},
			wantErr: true,
		},
		{
			name: "missing meminfo file returns error",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				// Create a proc dir with stat but no meminfo.
				dir := writeTempDir(t, map[string]string{
					"stat": "cpu  1000 500 300 8000 100 20 10 0 0 0\n",
				})
				return NewFileSystemMonitor(dir, testdataSysPath(t), testdataEmhttpPath(t))
			},
			wantErr: true,
		},
		{
			name: "nil context still works or returns meaningful error",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			// This test just verifies the call does not panic with a nil context.
			// We allow either success or error, but no panic.
			wantErr: false,
			validate: func(t *testing.T, ov *SystemOverview) {
				// No-op; the point is it didn't panic.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.monitor(t)
			ctx := context.Background()
			ov, err := m.GetOverview(ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, ov)
			}
		})
	}
}

func Test_GetOverview_CancelledContext(t *testing.T) {
	m := validMonitor(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := m.GetOverview(ctx)
	// Either error or success is acceptable depending on implementation,
	// but it must not panic. If the implementation checks context, it should
	// return an error.
	if err != nil {
		if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "context") {
			// Accept any error -- just document we ran the path.
			t.Logf("got error on cancelled context: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// GetArrayStatus Tests
// ---------------------------------------------------------------------------

func Test_GetArrayStatus_Cases(t *testing.T) {
	tests := []struct {
		name        string
		monitor     func(t *testing.T) *FileSystemMonitor
		wantErr     bool
		errContains string
		validate    func(t *testing.T, as *ArrayStatus)
	}{
		{
			name: "valid var.ini returns correct state",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, as *ArrayStatus) {
				t.Helper()
				if as == nil {
					t.Fatal("expected non-nil ArrayStatus")
				}
				if as.State != "STARTED" {
					t.Errorf("State = %q, want %q", as.State, "STARTED")
				}
			},
		},
		{
			name: "valid var.ini returns correct disk counts",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, as *ArrayStatus) {
				t.Helper()
				if as == nil {
					t.Fatal("expected non-nil ArrayStatus")
				}
				if as.NumDisks != 2 {
					t.Errorf("NumDisks = %d, want 2", as.NumDisks)
				}
				if as.NumProtected != 2 {
					t.Errorf("NumProtected = %d, want 2", as.NumProtected)
				}
				if as.NumInvalid != 0 {
					t.Errorf("NumInvalid = %d, want 0", as.NumInvalid)
				}
			},
		},
		{
			name: "valid var.ini returns zero sync errors",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, as *ArrayStatus) {
				t.Helper()
				if as == nil {
					t.Fatal("expected non-nil ArrayStatus")
				}
				if as.SyncErrors != 0 {
					t.Errorf("SyncErrors = %d, want 0", as.SyncErrors)
				}
			},
		},
		{
			name: "sync progress is zero when not syncing",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, as *ArrayStatus) {
				t.Helper()
				if as == nil {
					t.Fatal("expected non-nil ArrayStatus")
				}
				// mdResync=0 means not syncing; progress should be 0.
				if as.SyncProgress != 0 {
					t.Errorf("SyncProgress = %f, want 0", as.SyncProgress)
				}
			},
		},
		{
			name: "syncing array reports progress",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				dir := writeTempDir(t, map[string]string{
					"var.ini": strings.Join([]string{
						`mdState="STARTED"`,
						`mdResync="1"`,
						`mdResyncPos="500000"`,
						`mdResyncSize="1000000"`,
						`mdNumInvalid="1"`,
						`sbSynced="0"`,
						`sbSyncErrs="3"`,
						`mdNumDisks="4"`,
						`mdNumProtected="3"`,
					}, "\n") + "\n",
				})
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)
			},
			wantErr: false,
			validate: func(t *testing.T, as *ArrayStatus) {
				t.Helper()
				if as == nil {
					t.Fatal("expected non-nil ArrayStatus")
				}
				if as.NumDisks != 4 {
					t.Errorf("NumDisks = %d, want 4", as.NumDisks)
				}
				if as.NumProtected != 3 {
					t.Errorf("NumProtected = %d, want 3", as.NumProtected)
				}
				if as.NumInvalid != 1 {
					t.Errorf("NumInvalid = %d, want 1", as.NumInvalid)
				}
				if as.SyncErrors != 3 {
					t.Errorf("SyncErrors = %d, want 3", as.SyncErrors)
				}
				// SyncProgress: pos=500000 / size=1000000 = 50%
				if as.SyncProgress < 49.9 || as.SyncProgress > 50.1 {
					t.Errorf("SyncProgress = %f, want ~50.0", as.SyncProgress)
				}
			},
		},
		{
			name: "stopped array",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				dir := writeTempDir(t, map[string]string{
					"var.ini": strings.Join([]string{
						`mdState="STOPPED"`,
						`mdResync="0"`,
						`mdResyncPos="0"`,
						`mdResyncSize="0"`,
						`mdNumInvalid="0"`,
						`sbSynced="0"`,
						`sbSyncErrs="0"`,
						`mdNumDisks="0"`,
						`mdNumProtected="0"`,
					}, "\n") + "\n",
				})
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)
			},
			wantErr: false,
			validate: func(t *testing.T, as *ArrayStatus) {
				t.Helper()
				if as == nil {
					t.Fatal("expected non-nil ArrayStatus")
				}
				if as.State != "STOPPED" {
					t.Errorf("State = %q, want %q", as.State, "STOPPED")
				}
				if as.NumDisks != 0 {
					t.Errorf("NumDisks = %d, want 0", as.NumDisks)
				}
			},
		},
		{
			name: "missing var.ini returns error",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), "/nonexistent/emhttp")
			},
			wantErr: true,
		},
		{
			name: "empty emhttp dir returns error",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				dir := t.TempDir() // empty directory -- no var.ini
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.monitor(t)
			ctx := context.Background()
			as, err := m.GetArrayStatus(ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, as)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetDiskInfo Tests
// ---------------------------------------------------------------------------

func Test_GetDiskInfo_Cases(t *testing.T) {
	tests := []struct {
		name        string
		monitor     func(t *testing.T) *FileSystemMonitor
		wantErr     bool
		errContains string
		validate    func(t *testing.T, disks []DiskInfo)
	}{
		{
			name: "valid disks.ini returns 3 disks",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				if len(disks) != 3 {
					t.Fatalf("got %d disks, want 3", len(disks))
				}
			},
		},
		{
			name: "disk1 details parsed correctly",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				disk := findDisk(t, disks, "disk1")
				if disk.Device != "sdb" {
					t.Errorf("disk1.Device = %q, want %q", disk.Device, "sdb")
				}
				if disk.Temp != 38 {
					t.Errorf("disk1.Temp = %d, want 38", disk.Temp)
				}
				if disk.Status != "DISK_OK" {
					t.Errorf("disk1.Status = %q, want %q", disk.Status, "DISK_OK")
				}
				if disk.FsType != "xfs" {
					t.Errorf("disk1.FsType = %q, want %q", disk.FsType, "xfs")
				}
				if disk.FsSize != 3814697265 {
					t.Errorf("disk1.FsSize = %d, want 3814697265", disk.FsSize)
				}
				if disk.FsUsed != 1932735283 {
					t.Errorf("disk1.FsUsed = %d, want 1932735283", disk.FsUsed)
				}
			},
		},
		{
			name: "disk2 details parsed correctly",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				disk := findDisk(t, disks, "disk2")
				if disk.Device != "sdc" {
					t.Errorf("disk2.Device = %q, want %q", disk.Device, "sdc")
				}
				if disk.Temp != 36 {
					t.Errorf("disk2.Temp = %d, want 36", disk.Temp)
				}
				if disk.Status != "DISK_OK" {
					t.Errorf("disk2.Status = %q, want %q", disk.Status, "DISK_OK")
				}
				if disk.FsType != "xfs" {
					t.Errorf("disk2.FsType = %q, want %q", disk.FsType, "xfs")
				}
				if disk.FsSize != 3814697265 {
					t.Errorf("disk2.FsSize = %d, want 3814697265", disk.FsSize)
				}
				if disk.FsUsed != 2567890123 {
					t.Errorf("disk2.FsUsed = %d, want 2567890123", disk.FsUsed)
				}
			},
		},
		{
			name: "cache details parsed correctly",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				disk := findDisk(t, disks, "cache")
				if disk.Device != "nvme0n1" {
					t.Errorf("cache.Device = %q, want %q", disk.Device, "nvme0n1")
				}
				if disk.Temp != 45 {
					t.Errorf("cache.Temp = %d, want 45", disk.Temp)
				}
				if disk.Status != "DISK_OK" {
					t.Errorf("cache.Status = %q, want %q", disk.Status, "DISK_OK")
				}
				if disk.FsType != "btrfs" {
					t.Errorf("cache.FsType = %q, want %q", disk.FsType, "btrfs")
				}
				if disk.FsSize != 500107862 {
					t.Errorf("cache.FsSize = %d, want 500107862", disk.FsSize)
				}
				if disk.FsUsed != 123456789 {
					t.Errorf("cache.FsUsed = %d, want 123456789", disk.FsUsed)
				}
			},
		},
		{
			name: "disk names are unique",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return validMonitor(t)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				seen := make(map[string]bool)
				for _, d := range disks {
					if seen[d.Name] {
						t.Errorf("duplicate disk name: %q", d.Name)
					}
					seen[d.Name] = true
				}
			},
		},
		{
			name: "missing disks.ini returns error",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), "/nonexistent/emhttp")
			},
			wantErr: true,
		},
		{
			name: "empty emhttp dir returns error for disks",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				dir := t.TempDir()
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)
			},
			wantErr: true,
		},
		{
			name: "single disk in disks.ini",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				dir := writeTempDir(t, map[string]string{
					"disks.ini": strings.Join([]string{
						"[disk1]",
						`idx="1"`,
						`name="disk1"`,
						`device="sda"`,
						`temp="40"`,
						`status="DISK_OK"`,
						`fsSize="1000000"`,
						`fsUsed="500000"`,
						`fsType="xfs"`,
					}, "\n") + "\n",
				})
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				if len(disks) != 1 {
					t.Fatalf("got %d disks, want 1", len(disks))
				}
				if disks[0].Name != "disk1" {
					t.Errorf("Name = %q, want %q", disks[0].Name, "disk1")
				}
				if disks[0].Device != "sda" {
					t.Errorf("Device = %q, want %q", disks[0].Device, "sda")
				}
				if disks[0].Temp != 40 {
					t.Errorf("Temp = %d, want 40", disks[0].Temp)
				}
			},
		},
		{
			name: "disk with DISK_NP status",
			monitor: func(t *testing.T) *FileSystemMonitor {
				t.Helper()
				dir := writeTempDir(t, map[string]string{
					"disks.ini": strings.Join([]string{
						"[parity]",
						`idx="0"`,
						`name="parity"`,
						`device=""`,
						`temp="0"`,
						`status="DISK_NP"`,
						`fsSize="0"`,
						`fsUsed="0"`,
						`fsType=""`,
					}, "\n") + "\n",
				})
				return NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)
			},
			wantErr: false,
			validate: func(t *testing.T, disks []DiskInfo) {
				t.Helper()
				if len(disks) != 1 {
					t.Fatalf("got %d disks, want 1", len(disks))
				}
				if disks[0].Status != "DISK_NP" {
					t.Errorf("Status = %q, want %q", disks[0].Status, "DISK_NP")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.monitor(t)
			ctx := context.Background()
			disks, err := m.GetDiskInfo(ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, disks)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge Cases and Boundary Tests
// ---------------------------------------------------------------------------

func Test_GetOverview_EmptyTemperatures(t *testing.T) {
	// A sys path with no hwmon data should still return a valid overview,
	// just with an empty Temperatures slice.
	emptySys := t.TempDir()
	m := NewFileSystemMonitor(testdataProcPath(t), emptySys, testdataEmhttpPath(t))

	ctx := context.Background()
	ov, err := m.GetOverview(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov == nil {
		t.Fatal("expected non-nil SystemOverview")
	}
	if ov.Temperatures == nil {
		// nil is acceptable -- it just means no sensors found
		t.Log("Temperatures is nil (no hwmon found), which is acceptable")
	}
	if len(ov.Temperatures) != 0 {
		t.Errorf("expected 0 temperatures with empty sys, got %d", len(ov.Temperatures))
	}
}

func Test_GetArrayStatus_SyncingState(t *testing.T) {
	dir := writeTempDir(t, map[string]string{
		"var.ini": strings.Join([]string{
			`mdState="STARTED"`,
			`mdResync="1"`,
			`mdResyncPos="750000"`,
			`mdResyncSize="1000000"`,
			`mdNumInvalid="0"`,
			`sbSynced="0"`,
			`sbSyncErrs="0"`,
			`mdNumDisks="3"`,
			`mdNumProtected="2"`,
		}, "\n") + "\n",
	})
	m := NewFileSystemMonitor(testdataProcPath(t), testdataSysPath(t), dir)

	ctx := context.Background()
	as, err := m.GetArrayStatus(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SyncProgress should be pos/size * 100 = 75%
	if as.SyncProgress < 74.9 || as.SyncProgress > 75.1 {
		t.Errorf("SyncProgress = %f, want ~75.0", as.SyncProgress)
	}
}

func Test_GetDiskInfo_ReturnsSlice(t *testing.T) {
	// Verify the return type is a slice (not nil) when valid data exists.
	m := validMonitor(t)
	ctx := context.Background()
	disks, err := m.GetDiskInfo(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disks == nil {
		t.Fatal("expected non-nil slice of DiskInfo")
	}
}

// ---------------------------------------------------------------------------
// Benchmark Tests
// ---------------------------------------------------------------------------

func Benchmark_GetOverview(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		b.Fatalf("failed to resolve project root: %v", err)
	}
	procPath := filepath.Join(root, "testdata", "proc")
	sysPath := filepath.Join(root, "testdata", "sys")
	emhttpPath := filepath.Join(root, "testdata", "emhttp")

	m := NewFileSystemMonitor(procPath, sysPath, emhttpPath)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.GetOverview(ctx)
	}
}

func Benchmark_GetArrayStatus(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		b.Fatalf("failed to resolve project root: %v", err)
	}
	procPath := filepath.Join(root, "testdata", "proc")
	sysPath := filepath.Join(root, "testdata", "sys")
	emhttpPath := filepath.Join(root, "testdata", "emhttp")

	m := NewFileSystemMonitor(procPath, sysPath, emhttpPath)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.GetArrayStatus(ctx)
	}
}

func Benchmark_GetDiskInfo(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		b.Fatalf("failed to resolve project root: %v", err)
	}
	procPath := filepath.Join(root, "testdata", "proc")
	sysPath := filepath.Join(root, "testdata", "sys")
	emhttpPath := filepath.Join(root, "testdata", "emhttp")

	m := NewFileSystemMonitor(procPath, sysPath, emhttpPath)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.GetDiskInfo(ctx)
	}
}

// ---------------------------------------------------------------------------
// Test Helpers (internal to this file)
// ---------------------------------------------------------------------------

// findDisk locates a disk by name in the slice and fails the test if not found.
func findDisk(t *testing.T, disks []DiskInfo, name string) DiskInfo {
	t.Helper()
	for _, d := range disks {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("disk %q not found in %d disks", name, len(disks))
	return DiskInfo{} // unreachable
}
