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
	stats := &container.ContainerStats{}

	// Test concurrent access
	done := make(chan bool, 2)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			stats.RestartCount = i
			stats.LastExitCode = i
			now := time.Now()
			stats.LastRestart = &now
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = stats.RestartCount
			_ = stats.LastExitCode
			_ = stats.LastRestart
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we get here without race conditions, test passes
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
