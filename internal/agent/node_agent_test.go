package agent

import (
	"context"
	"testing"
	"time"
)

func TestDefaultNodeAgent_Start(t *testing.T) {
	tests := []struct {
		name    string
		nodeID  string
		address string
		wantErr bool
	}{
		{
			name:    "valid configuration",
			nodeID:  "test-node-1",
			address: "192.168.1.100:8081",
			wantErr: false,
		},
		{
			name:    "empty node ID",
			nodeID:  "",
			address: "192.168.1.100:8081",
			wantErr: true,
		},
		{
			name:    "empty address",
			nodeID:  "test-node-1",
			address: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := NewDefaultNodeAgent(tt.nodeID, tt.address, "http://localhost:8080")
			defer func() { _ = agent.Stop(context.Background()) }()

			ctx := context.Background()
			err := agent.Start(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				// Verify agent is running
				status, err := agent.GetNodeStatus(ctx)
				if err != nil {
					t.Errorf("GetNodeStatus() error = %v", err)
				}
				if status.NodeID != tt.nodeID {
					t.Errorf("Expected NodeID %s, got %s", tt.nodeID, status.NodeID)
				}
				if status.Address != tt.address {
					t.Errorf("Expected Address %s, got %s", tt.address, status.Address)
				}
			}
		})
	}
}

func TestDefaultNodeAgent_Stop(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	ctx := context.Background()

	// Start agent
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}

	// Stop agent
	err = agent.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify agent is stopped (should not panic or error)
	_, err = agent.GetNodeStatus(ctx)
	if err == nil {
		t.Error("Expected error when getting status of stopped agent")
	}
}

func TestDefaultNodeAgent_HandleContainerRequest_Create(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	defer func() { _ = agent.Stop(context.Background()) }()

	ctx := context.Background()
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}

	// Test create container request
	req := ContainerRequest{
		Operation: ContainerOpCreate,
		Config: &ContainerConfig{
			Image: "nginx:latest",
			Name:  "test-nginx-" + time.Now().Format("20060102-150405"),
		},
	}

	resp, err := agent.HandleContainerRequest(ctx, req)
	if err != nil {
		t.Errorf("HandleContainerRequest() error = %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected successful response, got error: %s", resp.Error)
	}

	if resp.ContainerID == "" {
		t.Error("Expected container ID in response")
	}

	// Clean up
	cleanupReq := ContainerRequest{
		Operation:   ContainerOpRemove,
		ContainerID: resp.ContainerID,
	}
	_, _ = agent.HandleContainerRequest(ctx, cleanupReq)
}

func TestDefaultNodeAgent_HandleContainerRequest_Start(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	defer func() { _ = agent.Stop(context.Background()) }()

	ctx := context.Background()
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}

	// First create a container
	createReq := ContainerRequest{
		Operation: ContainerOpCreate,
		Config: &ContainerConfig{
			Image: "nginx:latest",
			Name:  "test-nginx-" + time.Now().Format("20060102-150405"),
		},
	}

	createResp, err := agent.HandleContainerRequest(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Test start container request
	startReq := ContainerRequest{
		Operation:   ContainerOpStart,
		ContainerID: createResp.ContainerID,
	}

	startResp, err := agent.HandleContainerRequest(ctx, startReq)
	if err != nil {
		t.Errorf("HandleContainerRequest() error = %v", err)
	}

	if !startResp.Success {
		t.Errorf("Expected successful response, got error: %s", startResp.Error)
	}

	// Clean up
	stopReq := ContainerRequest{
		Operation:   ContainerOpStop,
		ContainerID: createResp.ContainerID,
		Timeout:     5 * time.Second,
	}
	_, _ = agent.HandleContainerRequest(ctx, stopReq)

	removeReq := ContainerRequest{
		Operation:   ContainerOpRemove,
		ContainerID: createResp.ContainerID,
	}
	_, _ = agent.HandleContainerRequest(ctx, removeReq)
}

func TestDefaultNodeAgent_HandleContainerRequest_Status(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	defer func() { _ = agent.Stop(context.Background()) }()

	ctx := context.Background()
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}

	// First create a container
	createReq := ContainerRequest{
		Operation: ContainerOpCreate,
		Config: &ContainerConfig{
			Image: "nginx:latest",
			Name:  "test-nginx-" + time.Now().Format("20060102-150405"),
		},
	}

	createResp, err := agent.HandleContainerRequest(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Test status request
	statusReq := ContainerRequest{
		Operation:   ContainerOpStatus,
		ContainerID: createResp.ContainerID,
	}

	statusResp, err := agent.HandleContainerRequest(ctx, statusReq)
	if err != nil {
		t.Errorf("HandleContainerRequest() error = %v", err)
	}

	if !statusResp.Success {
		t.Errorf("Expected successful response, got error: %s", statusResp.Error)
	}

	if statusResp.Status == nil {
		t.Error("Expected status in response")
	}

	if statusResp.Status.ID != createResp.ContainerID {
		t.Errorf("Expected status ID %s, got %s", createResp.ContainerID, statusResp.Status.ID)
	}

	// Clean up
	removeReq := ContainerRequest{
		Operation:   ContainerOpRemove,
		ContainerID: createResp.ContainerID,
	}
	_, _ = agent.HandleContainerRequest(ctx, removeReq)
}

func TestDefaultNodeAgent_HandleContainerRequest_InvalidOperation(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	defer func() { _ = agent.Stop(context.Background()) }()

	ctx := context.Background()
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}

	// Test invalid operation
	req := ContainerRequest{
		Operation: "invalid",
	}

	resp, err := agent.HandleContainerRequest(ctx, req)
	if err == nil {
		t.Error("Expected error for invalid operation")
	}

	if resp.Success {
		t.Error("Expected unsuccessful response for invalid operation")
	}
}

func TestDefaultNodeAgent_GetNodeStatus(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	defer func() { _ = agent.Stop(context.Background()) }()

	ctx := context.Background()
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent: %v", err)
	}

	status, err := agent.GetNodeStatus(ctx)
	if err != nil {
		t.Errorf("GetNodeStatus() error = %v", err)
	}

	if status.NodeID != "test-node-1" {
		t.Errorf("Expected NodeID test-node-1, got %s", status.NodeID)
	}

	if status.Address != "192.168.1.100:8081" {
		t.Errorf("Expected Address 192.168.1.100:8081, got %s", status.Address)
	}

	if status.Status != NodeStateHealthy {
		t.Errorf("Expected status healthy, got %s", status.Status)
	}

	if status.Version == "" {
		t.Error("Expected version to be set")
	}

	// Verify resource usage is included
	if status.Resources.Timestamp.IsZero() {
		t.Error("Expected resource timestamp to be set")
	}
}

func TestDefaultNodeAgent_DoubleStart(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")
	defer func() { _ = agent.Stop(context.Background()) }()

	ctx := context.Background()

	// Start first time
	err := agent.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start agent first time: %v", err)
	}

	// Try to start second time
	err = agent.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting agent twice")
	}
}

func TestDefaultNodeAgent_StopWithoutStart(t *testing.T) {
	agent := NewDefaultNodeAgent("test-node-1", "192.168.1.100:8081", "http://localhost:8080")

	ctx := context.Background()

	// Try to stop without starting
	err := agent.Stop(ctx)
	if err == nil {
		t.Error("Expected error when stopping agent that wasn't started")
	}
}
