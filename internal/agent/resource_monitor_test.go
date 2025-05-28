package agent

import (
	"context"
	"testing"
	"time"
)

func TestSystemResourceMonitor_GetResourceUsage(t *testing.T) {
	monitor := NewSystemResourceMonitor()

	ctx := context.Background()
	usage, err := monitor.GetResourceUsage(ctx)

	if err != nil {
		t.Fatalf("GetResourceUsage failed: %v", err)
	}

	if usage.CPUPercent < 0 || usage.CPUPercent > 100 {
		t.Errorf("Invalid CPU percentage: %f", usage.CPUPercent)
	}

	if usage.MemoryUsedMB < 0 || usage.MemoryTotalMB <= 0 {
		t.Errorf("Invalid memory values: used=%d, total=%d", usage.MemoryUsedMB, usage.MemoryTotalMB)
	}

	if usage.MemoryUsedMB > usage.MemoryTotalMB {
		t.Errorf("Memory used (%d) cannot exceed total (%d)", usage.MemoryUsedMB, usage.MemoryTotalMB)
	}

	if usage.DiskUsedMB < 0 || usage.DiskTotalMB <= 0 {
		t.Errorf("Invalid disk values: used=%d, total=%d", usage.DiskUsedMB, usage.DiskTotalMB)
	}

	if usage.NetworkRxMB < 0 || usage.NetworkTxMB < 0 {
		t.Errorf("Invalid network values: rx=%d, tx=%d", usage.NetworkRxMB, usage.NetworkTxMB)
	}

	if usage.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	timeDiff := time.Since(usage.Timestamp)
	if timeDiff > time.Second {
		t.Errorf("Timestamp too old: %v", timeDiff)
	}
}

func TestSystemResourceMonitor_StartStopMonitoring(t *testing.T) {
	monitor := NewSystemResourceMonitor()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.StartMonitoring(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("StartMonitoring failed: %v", err)
	}

	time.Sleep(250 * time.Millisecond)

	err = monitor.StopMonitoring()
	if err != nil {
		t.Fatalf("StopMonitoring failed: %v", err)
	}

	err = monitor.StopMonitoring()
	if err == nil {
		t.Error("Expected error when stopping already stopped monitor")
	}
}

func TestSystemResourceMonitor_StartMonitoringTwice(t *testing.T) {
	monitor := NewSystemResourceMonitor()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.StartMonitoring(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("First StartMonitoring failed: %v", err)
	}
	defer func() {
		if stopErr := monitor.StopMonitoring(); stopErr != nil {
			t.Logf("StopMonitoring error: %v", stopErr)
		}
	}()

	err = monitor.StartMonitoring(ctx, 100*time.Millisecond)
	if err == nil {
		t.Error("Expected error when starting already running monitor")
	}
}

func TestSystemResourceMonitor_CancelledContext(t *testing.T) {
	monitor := NewSystemResourceMonitor()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := monitor.GetResourceUsage(ctx)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

func TestSystemResourceMonitor_MonitoringWithCancelledContext(t *testing.T) {
	monitor := NewSystemResourceMonitor()

	ctx, cancel := context.WithCancel(context.Background())

	err := monitor.StartMonitoring(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("StartMonitoring failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	cancel()

	time.Sleep(100 * time.Millisecond)

	err = monitor.StopMonitoring()
	if err != nil {
		t.Fatalf("StopMonitoring failed: %v", err)
	}
}

func TestSystemResourceMonitor_ResourceConsistency(t *testing.T) {
	monitor := NewSystemResourceMonitor()

	ctx := context.Background()

	usage1, err := monitor.GetResourceUsage(ctx)
	if err != nil {
		t.Fatalf("First GetResourceUsage failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	usage2, err := monitor.GetResourceUsage(ctx)
	if err != nil {
		t.Fatalf("Second GetResourceUsage failed: %v", err)
	}

	if usage1.MemoryTotalMB != usage2.MemoryTotalMB {
		t.Errorf("Memory total should be consistent: %d vs %d", usage1.MemoryTotalMB, usage2.MemoryTotalMB)
	}

	if usage1.DiskTotalMB != usage2.DiskTotalMB {
		t.Errorf("Disk total should be consistent: %d vs %d", usage1.DiskTotalMB, usage2.DiskTotalMB)
	}

	if usage2.Timestamp.Before(usage1.Timestamp) {
		t.Error("Second timestamp should be after first timestamp")
	}
}
