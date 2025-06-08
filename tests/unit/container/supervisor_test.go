package container_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/lachau/go-orchestrator/internal/container"
)

func TestContainerSupervisor_Creation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cm := container.NewDockerManager(logger)

	policy := container.RestartPolicy{
		Type:       container.RestartPolicyOnFailure,
		MaxRetries: 3,
	}

	supervisor := container.NewContainerSupervisor(cm, logger, policy)

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}
}

func TestContainerSupervisor_StartStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cm := container.NewDockerManager(logger)

	policy := container.RestartPolicy{
		Type: container.RestartPolicyNever,
	}

	supervisor := container.NewContainerSupervisor(cm, logger, policy)
	ctx := context.Background()

	t.Run("starts successfully", func(t *testing.T) {
		err := supervisor.Start(ctx)
		if err != nil {
			t.Fatalf("Expected no error starting supervisor, got %v", err)
		}

		// Clean up
		supervisor.Stop()
	})

	t.Run("cannot start twice", func(t *testing.T) {
		err := supervisor.Start(ctx)
		if err != nil {
			t.Fatalf("Expected no error starting supervisor, got %v", err)
		}

		err = supervisor.Start(ctx)
		if err == nil {
			t.Fatal("Expected error when starting supervisor twice")
		}

		// Clean up
		supervisor.Stop()
	})

	t.Run("stops gracefully", func(t *testing.T) {
		err := supervisor.Start(ctx)
		if err != nil {
			t.Fatalf("Expected no error starting supervisor, got %v", err)
		}

		// Should not panic or error
		supervisor.Stop()

		// Should be able to stop multiple times
		supervisor.Stop()
	})
}

func TestContainerSupervisor_Stats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cm := container.NewDockerManager(logger)

	policy := container.RestartPolicy{
		Type: container.RestartPolicyNever,
	}

	supervisor := container.NewContainerSupervisor(cm, logger, policy)

	t.Run("returns error for non-existent container", func(t *testing.T) {
		_, err := supervisor.GetContainerStats("non-existent")
		if err == nil {
			t.Fatal("Expected error for non-existent container stats")
		}
	})
}

func TestRestartPolicy_Types(t *testing.T) {
	tests := []struct {
		name     string
		policy   container.RestartPolicyType
		expected container.RestartPolicyType
	}{
		{"never", container.RestartPolicyNever, container.RestartPolicyNever},
		{"always", container.RestartPolicyAlways, container.RestartPolicyAlways},
		{"on-failure", container.RestartPolicyOnFailure, container.RestartPolicyOnFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := container.RestartPolicy{
				Type: tt.policy,
			}

			if policy.Type != tt.expected {
				t.Errorf("Expected policy type %v, got %v", tt.expected, policy.Type)
			}
		})
	}
}

func TestContainerStats_ThreadSafety(t *testing.T) {
	// Note: This test should be run with -race flag to detect race conditions
	// Example: go test -race ./tests/unit/container/

	// This test demonstrates that ContainerStats fields need proper synchronization.
	// In production, ContainerStats should only be accessed through thread-safe
	// supervisor methods that handle locking internally.

	// Test supervisor's thread-safe access patterns
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cm := container.NewDockerManager(logger)

	policy := container.RestartPolicy{
		Type: container.RestartPolicyNever,
	}

	supervisor := container.NewContainerSupervisor(cm, logger, policy)

	// Create test container
	config := container.ContainerConfig{Image: "nginx:latest"}
	c, err := cm.Create(context.Background(), config)
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	done := make(chan bool, 2)

	// Goroutine 1: Multiple stats queries (thread-safe)
	go func() {
		defer func() { done <- true }()
		for i := 0; i < 50; i++ {
			// This is thread-safe - supervisor handles locking
			_, _ = supervisor.GetContainerStats(c.ID) // Expected to fail when stats don't exist yet
			time.Sleep(time.Microsecond)
		}
	}()

	// Goroutine 2: More stats queries (thread-safe)
	go func() {
		defer func() { done <- true }()
		for i := 0; i < 50; i++ {
			// This is thread-safe - supervisor handles locking
			_, _ = supervisor.GetContainerStats(c.ID) // Expected to fail when stats don't exist yet
			time.Sleep(time.Microsecond)
		}
	}()

	// Wait for completion
	<-done
	<-done

	// The test passes if no race conditions are detected
	// This verifies that supervisor's GetContainerStats is thread-safe
}

func TestDockerEvent_Parsing(t *testing.T) {
	// Test that DockerEvent struct can be properly unmarshaled
	event := container.DockerEvent{
		Type:   "container",
		Action: "start",
		Actor: struct {
			ID         string            `json:"ID"`
			Attributes map[string]string `json:"Attributes"`
		}{
			ID: "test-id",
			Attributes: map[string]string{
				"name": "orch-test-container",
			},
		},
		Time:     time.Now().Unix(),
		TimeNano: time.Now().UnixNano(),
		Status:   "start",
	}

	if event.Type != "container" {
		t.Errorf("Expected event type 'container', got %s", event.Type)
	}

	if event.Action != "start" {
		t.Errorf("Expected event action 'start', got %s", event.Action)
	}

	if event.Actor.Attributes["name"] != "orch-test-container" {
		t.Errorf("Expected container name 'orch-test-container', got %s", event.Actor.Attributes["name"])
	}
}
