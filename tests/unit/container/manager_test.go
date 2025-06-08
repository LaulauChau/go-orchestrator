package container_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/lachau/go-orchestrator/internal/container"
)

func TestDockerManager_Create(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	dm := container.NewDockerManager(logger)

	t.Run("creates container with valid config", func(t *testing.T) {
		config := container.ContainerConfig{
			Image: "nginx:latest",
			Env:   map[string]string{"ENV": "test"},
		}

		c, err := dm.Create(context.Background(), config)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if c.ID == "" {
			t.Error("Expected container ID to be set")
		}

		if c.Image != config.Image {
			t.Errorf("Expected image %s, got %s", config.Image, c.Image)
		}

		if c.GetState() != container.ContainerStateCreated {
			t.Errorf("Expected state %v, got %v", container.ContainerStateCreated, c.GetState())
		}

		if c.Created.IsZero() {
			t.Error("Expected Created timestamp to be set")
		}

		if c.Name == "" {
			t.Error("Expected container name to be generated")
		}
	})

	t.Run("fails with empty image", func(t *testing.T) {
		config := container.ContainerConfig{
			Image: "",
		}

		_, err := dm.Create(context.Background(), config)
		if err == nil {
			t.Fatal("Expected error for empty image")
		}
	})
}

func TestDockerManager_Get(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	dm := container.NewDockerManager(logger)

	config := container.ContainerConfig{Image: "nginx:latest"}
	c, _ := dm.Create(context.Background(), config)

	t.Run("gets existing container", func(t *testing.T) {
		retrieved, err := dm.Get(c.ID)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if retrieved.ID != c.ID {
			t.Errorf("Expected ID %s, got %s", c.ID, retrieved.ID)
		}
	})

	t.Run("fails for non-existent container", func(t *testing.T) {
		_, err := dm.Get("non-existent")
		if err != container.ErrContainerNotFound {
			t.Errorf("Expected %v, got %v", container.ErrContainerNotFound, err)
		}
	})
}

func TestDockerManager_List(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	dm := container.NewDockerManager(logger)

	t.Run("lists empty containers initially", func(t *testing.T) {
		containers, err := dm.List()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(containers) != 0 {
			t.Errorf("Expected 0 containers, got %d", len(containers))
		}
	})

	t.Run("lists created containers", func(t *testing.T) {
		config := container.ContainerConfig{Image: "nginx:latest"}
		container1, _ := dm.Create(context.Background(), config)
		container2, _ := dm.Create(context.Background(), config)

		containers, err := dm.List()
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if len(containers) != 2 {
			t.Errorf("Expected 2 containers, got %d", len(containers))
		}

		// Check that both containers are present
		ids := make(map[string]bool)
		for _, c := range containers {
			ids[c.ID] = true
		}

		if !ids[container1.ID] {
			t.Errorf("Container %s not found in list", container1.ID)
		}
		if !ids[container2.ID] {
			t.Errorf("Container %s not found in list", container2.ID)
		}
	})
}

func TestDockerManager_StateTransitions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	dm := container.NewDockerManager(logger)

	config := container.ContainerConfig{Image: "nginx:latest"}
	c, _ := dm.Create(context.Background(), config)

	t.Run("cannot start non-existent container", func(t *testing.T) {
		err := dm.Start(context.Background(), "non-existent")
		if err != container.ErrContainerNotFound {
			t.Errorf("Expected %v, got %v", container.ErrContainerNotFound, err)
		}
	})

	t.Run("cannot stop non-running container", func(t *testing.T) {
		err := dm.Stop(context.Background(), c.ID)
		if err != container.ErrContainerStopped {
			t.Errorf("Expected %v, got %v", container.ErrContainerStopped, err)
		}
	})

	t.Run("cannot delete non-existent container", func(t *testing.T) {
		err := dm.Delete(context.Background(), "non-existent")
		if err != container.ErrContainerNotFound {
			t.Errorf("Expected %v, got %v", container.ErrContainerNotFound, err)
		}
	})
}
