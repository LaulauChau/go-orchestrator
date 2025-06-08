package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrContainerNotFound = errors.New("container not found")
	ErrContainerRunning  = errors.New("container is already running")
	ErrContainerStopped  = errors.New("container is not running")
)

// ContainerManager defines the interface for container operations
type ContainerManager interface {
	Create(ctx context.Context, config ContainerConfig) (*Container, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string) error
	Delete(ctx context.Context, containerID string) error
	Get(containerID string) (*Container, error)
	List() ([]*Container, error)
}

// dockerManager implements ContainerManager using Docker CLI
type dockerManager struct {
	containers map[string]*Container
	mu         sync.RWMutex
	logger     *slog.Logger
}

// NewDockerManager creates a new Docker-based container manager
func NewDockerManager(logger *slog.Logger) ContainerManager {
	return &dockerManager{
		containers: make(map[string]*Container),
		logger:     logger,
	}
}

// Create creates a new container with the given configuration
func (dm *dockerManager) Create(ctx context.Context, config ContainerConfig) (*Container, error) {
	if config.Image == "" {
		return nil, errors.New("image is required")
	}

	container := &Container{
		ID:      uuid.New().String(),
		Name:    generateContainerName(config.Image),
		Image:   config.Image,
		State:   ContainerStateCreated,
		Config:  config,
		Created: time.Now(),
	}

	dm.mu.Lock()
	dm.containers[container.ID] = container
	dm.mu.Unlock()

	dm.logger.Info("container created",
		slog.String("id", container.ID),
		slog.String("name", container.Name),
		slog.String("image", container.Image))

	return container, nil
}

// Start starts a container using Docker CLI
func (dm *dockerManager) Start(ctx context.Context, containerID string) error {
	dm.mu.Lock()
	container, exists := dm.containers[containerID]
	if !exists {
		dm.mu.Unlock()
		return ErrContainerNotFound
	}

	if container.IsRunning() {
		dm.mu.Unlock()
		return ErrContainerRunning
	}
	dm.mu.Unlock()

	// Build docker run command
	args := dm.buildDockerArgs(container)

	// #nosec G204 - controlled docker execution is required for container orchestration
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		container.SetState(ContainerStateFailed)
		return fmt.Errorf("failed to start container: %w, output: %s", err, string(output))
	}

	container.SetState(ContainerStateRunning)

	dm.logger.Info("container started",
		slog.String("id", container.ID),
		slog.String("name", container.Name))

	return nil
}

// Stop stops a running container
func (dm *dockerManager) Stop(ctx context.Context, containerID string) error {
	dm.mu.Lock()
	container, exists := dm.containers[containerID]
	if !exists {
		dm.mu.Unlock()
		return ErrContainerNotFound
	}

	if !container.IsRunning() {
		dm.mu.Unlock()
		return ErrContainerStopped
	}
	dm.mu.Unlock()

	// Stop the container using docker stop
	// #nosec G204 - controlled docker execution is required for container orchestration
	cmd := exec.CommandContext(ctx, "docker", "stop", container.Name)
	if err := cmd.Run(); err != nil {
		container.SetState(ContainerStateFailed)
		return fmt.Errorf("failed to stop container: %w", err)
	}

	container.SetState(ContainerStateStopped)

	dm.logger.Info("container stopped",
		slog.String("id", container.ID),
		slog.String("name", container.Name))

	return nil
}

// Delete removes a container
func (dm *dockerManager) Delete(ctx context.Context, containerID string) error {
	dm.mu.Lock()
	container, exists := dm.containers[containerID]
	if !exists {
		dm.mu.Unlock()
		return ErrContainerNotFound
	}

	// Stop container if running
	if container.IsRunning() {
		dm.mu.Unlock()
		if err := dm.Stop(ctx, containerID); err != nil {
			return fmt.Errorf("failed to stop container before deletion: %w", err)
		}
		dm.mu.Lock()
	}

	delete(dm.containers, containerID)
	dm.mu.Unlock()

	// Remove container using docker rm
	// #nosec G204 - controlled docker execution is required for container orchestration
	cmd := exec.CommandContext(ctx, "docker", "rm", container.Name)
	if err := cmd.Run(); err != nil {
		dm.logger.Warn("failed to remove docker container",
			slog.String("id", container.ID),
			slog.String("error", err.Error()))
	}

	dm.logger.Info("container deleted",
		slog.String("id", container.ID),
		slog.String("name", container.Name))

	return nil
}

// Get retrieves a container by ID
func (dm *dockerManager) Get(containerID string) (*Container, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	container, exists := dm.containers[containerID]
	if !exists {
		return nil, ErrContainerNotFound
	}

	return container, nil
}

// List returns all containers
func (dm *dockerManager) List() ([]*Container, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	containers := make([]*Container, 0, len(dm.containers))
	for _, container := range dm.containers {
		containers = append(containers, container)
	}

	return containers, nil
}

// buildDockerArgs constructs docker run arguments from container config
func (dm *dockerManager) buildDockerArgs(container *Container) []string {
	args := []string{"run", "-d", "--name", container.Name}

	config := container.Config

	// Add environment variables
	for key, value := range config.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add port mappings
	for _, port := range config.Ports {
		portMapping := fmt.Sprintf("%d:%d", port.HostPort, port.ContainerPort)
		if port.Protocol != "" && port.Protocol != "tcp" {
			portMapping += "/" + port.Protocol
		}
		args = append(args, "-p", portMapping)
	}

	// Add volume mounts
	for _, volume := range config.Volumes {
		volumeMapping := fmt.Sprintf("%s:%s", volume.Source, volume.Destination)
		if volume.ReadOnly {
			volumeMapping += ":ro"
		}
		args = append(args, "-v", volumeMapping)
	}

	// Add working directory
	if config.WorkingDir != "" {
		args = append(args, "-w", config.WorkingDir)
	}

	// Add image
	args = append(args, config.Image)

	// Add command and args
	if len(config.Command) > 0 {
		args = append(args, config.Command...)
	}
	if len(config.Args) > 0 {
		args = append(args, config.Args...)
	}

	return args
}

// generateContainerName creates a unique container name from image
func generateContainerName(image string) string {
	// Extract image name without tag/registry
	parts := strings.Split(image, "/")
	imageName := parts[len(parts)-1]

	if strings.Contains(imageName, ":") {
		imageName = strings.Split(imageName, ":")[0]
	}

	// Add random suffix
	suffix := uuid.New().String()[:8]
	return fmt.Sprintf("orch-%s-%s", imageName, suffix)
}
