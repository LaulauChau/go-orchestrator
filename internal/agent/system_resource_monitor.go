package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SystemResourceMonitor struct {
	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	lastCPU cpuStats
}

type cpuStats struct {
	user   uint64
	nice   uint64
	system uint64
	idle   uint64
	total  uint64
}

func NewSystemResourceMonitor() ResourceMonitor {
	return &SystemResourceMonitor{
		stopCh: make(chan struct{}),
	}
}

func (s *SystemResourceMonitor) GetResourceUsage(ctx context.Context) (ResourceUsage, error) {
	if ctx.Err() != nil {
		return ResourceUsage{}, ctx.Err()
	}

	usage := ResourceUsage{
		Timestamp: time.Now(),
	}

	var err error

	usage.CPUPercent, err = s.getCPUUsage()
	if err != nil {
		return ResourceUsage{}, fmt.Errorf("failed to get CPU usage: %w", err)
	}

	usage.MemoryUsedMB, usage.MemoryTotalMB, err = s.getMemoryUsage()
	if err != nil {
		return ResourceUsage{}, fmt.Errorf("failed to get memory usage: %w", err)
	}

	usage.DiskUsedMB, usage.DiskTotalMB, err = s.getDiskUsage()
	if err != nil {
		return ResourceUsage{}, fmt.Errorf("failed to get disk usage: %w", err)
	}

	usage.NetworkRxMB, usage.NetworkTxMB, err = s.getNetworkUsage()
	if err != nil {
		return ResourceUsage{}, fmt.Errorf("failed to get network usage: %w", err)
	}

	return usage, nil
}

func (s *SystemResourceMonitor) StartMonitoring(ctx context.Context, interval time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("monitoring is already running")
	}

	s.running = true
	s.stopCh = make(chan struct{})

	go s.monitoringLoop(ctx, interval)

	return nil
}

func (s *SystemResourceMonitor) StopMonitoring() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("monitoring is not running")
	}

	s.running = false
	close(s.stopCh)

	return nil
}

func (s *SystemResourceMonitor) monitoringLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
		}
	}
}

func (s *SystemResourceMonitor) getCPUUsage() (float64, error) {
	if runtime.GOOS != "linux" {
		return s.getCPUUsageGeneric()
	}

	return s.getCPUUsageLinux()
}

func (s *SystemResourceMonitor) getCPUUsageGeneric() (float64, error) {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return float64(runtime.NumGoroutine()) / float64(runtime.NumCPU()) * 10.0, nil
}

func (s *SystemResourceMonitor) getCPUUsageLinux() (float64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, fmt.Errorf("failed to read /proc/stat")
	}

	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, fmt.Errorf("invalid /proc/stat format")
	}

	current := cpuStats{}
	current.user, _ = strconv.ParseUint(fields[1], 10, 64)
	current.nice, _ = strconv.ParseUint(fields[2], 10, 64)
	current.system, _ = strconv.ParseUint(fields[3], 10, 64)
	current.idle, _ = strconv.ParseUint(fields[4], 10, 64)
	current.total = current.user + current.nice + current.system + current.idle

	if s.lastCPU.total == 0 {
		s.lastCPU = current
		return 0.0, nil
	}

	totalDiff := current.total - s.lastCPU.total
	idleDiff := current.idle - s.lastCPU.idle

	if totalDiff == 0 {
		return 0.0, nil
	}

	cpuPercent := float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
	s.lastCPU = current

	return cpuPercent, nil
}

func (s *SystemResourceMonitor) getMemoryUsage() (int64, int64, error) {
	if runtime.GOOS != "linux" {
		return s.getMemoryUsageGeneric()
	}

	return s.getMemoryUsageLinux()
}

func (s *SystemResourceMonitor) getMemoryUsageGeneric() (int64, int64, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// #nosec G115 - System memory values are safe for int64 conversion
	used := int64(m.Alloc) / (1024 * 1024)
	// #nosec G115 - System memory values are safe for int64 conversion
	total := int64(m.Sys) / (1024 * 1024)

	if total < used {
		total = used * 2
	}

	return used, total, nil
}

func (s *SystemResourceMonitor) getMemoryUsageLinux() (int64, int64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = file.Close() }()

	var memTotal, memFree, memAvailable int64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			memTotal = value / 1024
		case "MemFree:":
			memFree = value / 1024
		case "MemAvailable:":
			memAvailable = value / 1024
		}
	}

	if memTotal == 0 {
		return 0, 0, fmt.Errorf("failed to parse memory info")
	}

	memUsed := memTotal - memFree
	if memAvailable > 0 {
		memUsed = memTotal - memAvailable
	}

	return memUsed, memTotal, nil
}

func (s *SystemResourceMonitor) getDiskUsage() (int64, int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs("/", &stat)
	if err != nil {
		return 0, 0, err
	}

	// #nosec G115 - Filesystem values are safe for int64 conversion
	total := int64(stat.Blocks) * int64(stat.Bsize) / (1024 * 1024)
	// #nosec G115 - Filesystem values are safe for int64 conversion
	free := int64(stat.Bavail) * int64(stat.Bsize) / (1024 * 1024)
	used := total - free

	return used, total, nil
}

func (s *SystemResourceMonitor) getNetworkUsage() (int64, int64, error) {
	if runtime.GOOS != "linux" {
		return 0, 0, nil
	}

	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = file.Close() }()

	var totalRx, totalTx int64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}

		rxBytes, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}

		txBytes, err := strconv.ParseInt(fields[8], 10, 64)
		if err != nil {
			continue
		}

		totalRx += rxBytes / (1024 * 1024)
		totalTx += txBytes / (1024 * 1024)
	}

	return totalRx, totalTx, nil
}
