package agent

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDockerContainerManager_CreateContainer(t *testing.T) {
	timestamp := time.Now().Unix()

	tests := []struct {
		name    string
		config  ContainerConfig
		wantErr bool
	}{
		{
			name: "valid nginx container",
			config: ContainerConfig{
				Image: "nginx:alpine",
				Name:  fmt.Sprintf("test-nginx-%d", timestamp),
				Ports: []PortMapping{
					{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"},
				},
				Resources: ResourceLimits{
					CPUMillicores: 500,
					MemoryMB:      256,
				},
			},
			wantErr: false,
		},
		{
			name: "container with environment variables",
			config: ContainerConfig{
				Image:   "alpine:latest",
				Name:    fmt.Sprintf("test-alpine-%d", timestamp),
				Command: []string{"sh", "-c", "echo $TEST_VAR && sleep 10"},
				Environment: map[string]string{
					"TEST_VAR": "hello-world",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid image name",
			config: ContainerConfig{
				Image: "nonexistent-image:invalid-tag",
				Name:  fmt.Sprintf("test-invalid-%d", timestamp),
			},
			wantErr: true,
		},
		{
			name: "empty image name",
			config: ContainerConfig{
				Image: "",
				Name:  fmt.Sprintf("test-empty-image-%d", timestamp),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := NewDockerContainerManager()
			ctx := context.Background()

			containerID, err := cm.CreateContainer(ctx, tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("CreateContainer() unexpected error: %v", err)
				return
			}

			if containerID == "" {
				t.Errorf("CreateContainer() returned empty container ID")
			}

			defer func() {
				_ = cm.RemoveContainer(ctx, containerID)
			}()

			status, err := cm.GetContainerStatus(ctx, containerID)
			if err != nil {
				t.Errorf("GetContainerStatus() failed: %v", err)
				return
			}

			if status.State != ContainerStateCreated {
				t.Errorf("Expected container state %v, got %v", ContainerStateCreated, status.State)
			}
		})
	}
}

func TestDockerContainerManager_StartContainer(t *testing.T) {
	cm := NewDockerContainerManager()
	ctx := context.Background()
	timestamp := time.Now().Unix()

	config := ContainerConfig{
		Image:   "alpine:latest",
		Name:    fmt.Sprintf("test-start-container-%d", timestamp),
		Command: []string{"sleep", "30"},
	}

	containerID, err := cm.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	defer func() {
		_ = cm.StopContainer(ctx, containerID, 5*time.Second)
		_ = cm.RemoveContainer(ctx, containerID)
	}()

	err = cm.StartContainer(ctx, containerID)
	if err != nil {
		t.Errorf("StartContainer() failed: %v", err)
		return
	}

	status, err := cm.GetContainerStatus(ctx, containerID)
	if err != nil {
		t.Errorf("GetContainerStatus() failed: %v", err)
		return
	}

	if status.State != ContainerStateRunning {
		t.Errorf("Expected container state %v, got %v", ContainerStateRunning, status.State)
	}

	err = cm.StartContainer(ctx, containerID)
	if err != nil {
		t.Errorf("StartContainer() on running container failed: %v", err)
	}
}

func TestDockerContainerManager_StopContainer(t *testing.T) {
	cm := NewDockerContainerManager()
	ctx := context.Background()
	timestamp := time.Now().Unix()

	config := ContainerConfig{
		Image:   "alpine:latest",
		Name:    fmt.Sprintf("test-stop-container-%d", timestamp),
		Command: []string{"sleep", "60"},
	}

	containerID, err := cm.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}
	defer func() {
		_ = cm.RemoveContainer(ctx, containerID)
	}()

	err = cm.StartContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	err = cm.StopContainer(ctx, containerID, 10*time.Second)
	if err != nil {
		t.Errorf("StopContainer() failed: %v", err)
		return
	}

	status, err := cm.GetContainerStatus(ctx, containerID)
	if err != nil {
		t.Errorf("GetContainerStatus() failed: %v", err)
		return
	}

	if status.State != ContainerStateStopped && status.State != ContainerStateExited {
		t.Errorf("Expected container state stopped or exited, got %v", status.State)
	}
}

func TestDockerContainerManager_RemoveContainer(t *testing.T) {
	cm := NewDockerContainerManager()
	ctx := context.Background()
	timestamp := time.Now().Unix()

	config := ContainerConfig{
		Image: "alpine:latest",
		Name:  fmt.Sprintf("test-remove-container-%d", timestamp),
	}

	containerID, err := cm.CreateContainer(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	err = cm.RemoveContainer(ctx, containerID)
	if err != nil {
		t.Errorf("RemoveContainer() failed: %v", err)
		return
	}

	_, err = cm.GetContainerStatus(ctx, containerID)
	if err == nil {
		t.Errorf("Expected error when getting status of removed container")
	}
}

func TestDockerContainerManager_ListContainers(t *testing.T) {
	cm := NewDockerContainerManager()
	ctx := context.Background()
	timestamp := time.Now().Unix()

	configs := []ContainerConfig{
		{Image: "alpine:latest", Name: fmt.Sprintf("test-list-1-%d", timestamp)},
		{Image: "alpine:latest", Name: fmt.Sprintf("test-list-2-%d", timestamp)},
	}

	var containerIDs []string
	for _, config := range configs {
		containerID, err := cm.CreateContainer(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create container: %v", err)
		}
		containerIDs = append(containerIDs, containerID)
	}

	defer func() {
		for _, id := range containerIDs {
			_ = cm.RemoveContainer(ctx, id)
		}
	}()

	containers, err := cm.ListContainers(ctx)
	if err != nil {
		t.Errorf("ListContainers() failed: %v", err)
		return
	}

	found := 0
	for _, container := range containers {
		for _, id := range containerIDs {
			if container.ID == id || container.ID == id[:12] || id == container.ID {
				found++
				break
			}
		}
	}

	if found != len(containerIDs) {
		t.Errorf("Expected to find %d containers, found %d", len(containerIDs), found)
	}
}

func TestDockerContainerManager_GetContainerStatus_NotFound(t *testing.T) {
	cm := NewDockerContainerManager()
	ctx := context.Background()

	_, err := cm.GetContainerStatus(ctx, "nonexistent-container-id")
	if err == nil {
		t.Errorf("Expected error for nonexistent container")
	}
}
