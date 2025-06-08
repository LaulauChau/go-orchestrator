//go:build integration

package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/lachau/go-orchestrator/internal/container"
)

func TestContainerSupervisor_RealDockerIntegration(t *testing.T) {
	// Skip if Docker is not available
	if os.Getenv("SKIP_DOCKER") == "true" {
		t.Skip("Skipping Docker integration test")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cm := container.NewDockerManager(logger)

	policy := container.RestartPolicy{
		Type:       container.RestartPolicyOnFailure,
		MaxRetries: 2,
	}

	supervisor := container.NewContainerSupervisor(cm, logger, policy)
	ctx := context.Background()

	// Start supervisor
	err := supervisor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start supervisor: %v", err)
	}
	defer supervisor.Stop()

	t.Run("monitors container lifecycle", func(t *testing.T) {
		// Create a short-lived container
		config := container.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sleep"},
			Args:    []string{"2"}, // Sleep for 2 seconds
		}

		// Create container
		c, err := cm.Create(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create container: %v", err)
		}

		// Start container
		err = cm.Start(ctx, c.ID)
		if err != nil {
			t.Fatalf("Failed to start container: %v", err)
		}

		// Wait for container to start and be detected by supervisor
		time.Sleep(1 * time.Second)

		// Check if container is running
		if c.GetState() != container.ContainerStateRunning {
			t.Errorf("Expected container to be running, got %s", c.GetState())
		}

		// Wait for container to exit
		time.Sleep(3 * time.Second)

		// Check if supervisor detected the exit
		if c.GetState() != container.ContainerStateStopped && c.GetState() != container.ContainerStateFailed {
			t.Errorf("Expected container to be stopped or failed, got %s", c.GetState())
		}

		// Clean up
		cm.Delete(ctx, c.ID)
	})

	t.Run("tracks container statistics", func(t *testing.T) {
		// Create a container that will fail
		config := container.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sh"},
			Args:    []string{"-c", "exit 1"}, // Exit with code 1
		}

		// Create container
		c, err := cm.Create(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create container: %v", err)
		}

		// Start container
		err = cm.Start(ctx, c.ID)
		if err != nil {
			t.Fatalf("Failed to start container: %v", err)
		}

		// Wait for container to exit and restart attempts
		time.Sleep(5 * time.Second)

		// Check stats
		stats, err := supervisor.GetContainerStats(c.ID)
		if err != nil {
			t.Fatalf("Failed to get container stats: %v", err)
		}

		// Should have attempted restarts due to failure
		if stats.RestartCount == 0 {
			t.Error("Expected at least one restart attempt")
		}

		if stats.LastExitCode != 1 {
			t.Errorf("Expected last exit code to be 1, got %d", stats.LastExitCode)
		}

		// Clean up
		cm.Delete(ctx, c.ID)
	})
}

func TestContainerSupervisor_RestartPolicies(t *testing.T) {
	if os.Getenv("SKIP_DOCKER") == "true" {
		t.Skip("Skipping Docker integration test")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cm := container.NewDockerManager(logger)
	ctx := context.Background()

	t.Run("never restart policy", func(t *testing.T) {
		policy := container.RestartPolicy{
			Type: container.RestartPolicyNever,
		}

		supervisor := container.NewContainerSupervisor(cm, logger, policy)
		err := supervisor.Start(ctx)
		if err != nil {
			t.Fatalf("Failed to start supervisor: %v", err)
		}
		defer supervisor.Stop()

		// Create failing container
		config := container.ContainerConfig{
			Image:   "alpine:latest",
			Command: []string{"sh"},
			Args:    []string{"-c", "exit 1"},
		}

		c, err := cm.Create(ctx, config)
		if err != nil {
			t.Fatalf("Failed to create container: %v", err)
		}

		err = cm.Start(ctx, c.ID)
		if err != nil {
			t.Fatalf("Failed to start container: %v", err)
		}

		// Wait for exit
		time.Sleep(3 * time.Second)

		// Should not restart
		stats, _ := supervisor.GetContainerStats(c.ID)
		if stats != nil && stats.RestartCount > 0 {
			t.Errorf("Expected no restarts with never policy, got %d", stats.RestartCount)
		}

		cm.Delete(ctx, c.ID)
	})
}
