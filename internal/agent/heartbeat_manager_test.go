package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestHTTPHeartbeatManager_StartHeartbeat(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantErr  bool
	}{
		{
			name:     "valid interval",
			interval: 1 * time.Second,
			wantErr:  false,
		},
		{
			name:     "zero interval",
			interval: 0,
			wantErr:  true,
		},
		{
			name:     "negative interval",
			interval: -1 * time.Second,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewHTTPHeartbeatManager("http://localhost:8080")
			defer func() { _ = manager.StopHeartbeat() }()

			ctx := context.Background()
			err := manager.StartHeartbeat(ctx, tt.interval)

			if (err != nil) != tt.wantErr {
				t.Errorf("StartHeartbeat() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify heartbeat is running
				time.Sleep(100 * time.Millisecond)
				if !manager.IsRunning() {
					t.Error("Expected heartbeat to be running")
				}
			}
		})
	}
}

func TestHTTPHeartbeatManager_StopHeartbeat(t *testing.T) {
	manager := NewHTTPHeartbeatManager("http://localhost:8080")
	ctx := context.Background()

	// Start heartbeat
	err := manager.StartHeartbeat(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to start heartbeat: %v", err)
	}

	// Verify it's running
	if !manager.IsRunning() {
		t.Error("Expected heartbeat to be running")
	}

	// Stop heartbeat
	err = manager.StopHeartbeat()
	if err != nil {
		t.Errorf("StopHeartbeat() error = %v", err)
	}

	// Verify it's stopped
	time.Sleep(100 * time.Millisecond)
	if manager.IsRunning() {
		t.Error("Expected heartbeat to be stopped")
	}
}

func TestHTTPHeartbeatManager_SendHeartbeat(t *testing.T) {
	manager := NewHTTPHeartbeatManager("http://localhost:8080")
	ctx := context.Background()

	status := NodeStatus{
		NodeID:  "test-node-1",
		Address: "192.168.1.100",
		Status:  NodeStateHealthy,
		Resources: ResourceUsage{
			CPUPercent:    25.5,
			MemoryUsedMB:  1024,
			MemoryTotalMB: 4096,
			Timestamp:     time.Now(),
		},
		LastSeen: time.Now(),
		Version:  "v0.1.0",
	}

	// Test sending heartbeat (will fail due to no server, but should not panic)
	err := manager.SendHeartbeat(ctx, status)

	// We expect an error since there's no server running
	if err == nil {
		t.Error("Expected error when sending heartbeat to non-existent server")
	}
}

func TestHTTPHeartbeatManager_DoubleStart(t *testing.T) {
	manager := NewHTTPHeartbeatManager("http://localhost:8080")
	defer func() { _ = manager.StopHeartbeat() }()

	ctx := context.Background()

	// Start first heartbeat
	err := manager.StartHeartbeat(ctx, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to start first heartbeat: %v", err)
	}

	// Try to start second heartbeat
	err = manager.StartHeartbeat(ctx, 1*time.Second)
	if err == nil {
		t.Error("Expected error when starting heartbeat twice")
	}
}

func TestHTTPHeartbeatManager_StopWithoutStart(t *testing.T) {
	manager := NewHTTPHeartbeatManager("http://localhost:8080")

	// Try to stop without starting
	err := manager.StopHeartbeat()
	if err == nil {
		t.Error("Expected error when stopping heartbeat that wasn't started")
	}
}

func TestHTTPHeartbeatManager_ContextCancellation(t *testing.T) {
	manager := NewHTTPHeartbeatManager("http://localhost:8080")
	defer func() { _ = manager.StopHeartbeat() }()

	ctx, cancel := context.WithCancel(context.Background())

	// Start heartbeat
	err := manager.StartHeartbeat(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to start heartbeat: %v", err)
	}

	// Verify it's running
	if !manager.IsRunning() {
		t.Error("Expected heartbeat to be running")
	}

	// Cancel context
	cancel()

	// Wait for heartbeat to stop due to context cancellation
	time.Sleep(200 * time.Millisecond)

	if manager.IsRunning() {
		t.Error("Expected heartbeat to stop after context cancellation")
	}
}

func TestHTTPHeartbeatManager_ConcurrentOperations(t *testing.T) {
	manager := NewHTTPHeartbeatManager("http://localhost:8080")
	defer func() { _ = manager.StopHeartbeat() }()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Start heartbeat
	err := manager.StartHeartbeat(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to start heartbeat: %v", err)
	}

	// Concurrent heartbeat sends
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(nodeID int) {
			defer wg.Done()
			status := NodeStatus{
				NodeID:   fmt.Sprintf("test-node-%d", nodeID),
				Address:  "192.168.1.100",
				Status:   NodeStateHealthy,
				LastSeen: time.Now(),
			}
			err := manager.SendHeartbeat(ctx, status)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any unexpected errors (connection errors are expected)
	for err := range errors {
		// We expect connection errors since there's no server
		// Just verify no panics or race conditions occurred
		if err == nil {
			t.Error("Unexpected success when no server is running")
		}
	}
}

func TestHTTPHeartbeatManager_InvalidURL(t *testing.T) {
	manager := NewHTTPHeartbeatManager("invalid-url")
	ctx := context.Background()

	status := NodeStatus{
		NodeID:   "test-node-1",
		Address:  "192.168.1.100",
		Status:   NodeStateHealthy,
		LastSeen: time.Now(),
	}

	err := manager.SendHeartbeat(ctx, status)
	if err == nil {
		t.Error("Expected error when using invalid URL")
	}
}
