package container

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RestartPolicy defines how containers should be restarted
type RestartPolicy struct {
	Type       RestartPolicyType `json:"type"`
	MaxRetries int               `json:"max_retries,omitempty"`
}

type RestartPolicyType string

const (
	RestartPolicyNever     RestartPolicyType = "never"
	RestartPolicyAlways    RestartPolicyType = "always"
	RestartPolicyOnFailure RestartPolicyType = "on-failure"
)

// ContainerStats holds runtime statistics for a container
type ContainerStats struct {
	mu           sync.RWMutex
	RestartCount int           `json:"restart_count"`
	LastExitCode int           `json:"last_exit_code,omitempty"`
	Uptime       time.Duration `json:"uptime"`
	LastRestart  *time.Time    `json:"last_restart,omitempty"`
}

// DockerEvent represents a Docker event from the events stream
type DockerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	Time     int64  `json:"time"`
	TimeNano int64  `json:"timeNano"`
	Status   string `json:"status"`
}

// ContainerSupervisor monitors container processes and handles lifecycle events
type ContainerSupervisor struct {
	manager       ContainerManager
	logger        *slog.Logger
	eventsChan    chan DockerEvent
	stopChan      chan struct{}
	restartPolicy RestartPolicy
	stats         map[string]*ContainerStats
	statsMutex    sync.RWMutex
	running       bool
	runningMutex  sync.RWMutex
}

// NewContainerSupervisor creates a new container supervisor
func NewContainerSupervisor(manager ContainerManager, logger *slog.Logger, policy RestartPolicy) *ContainerSupervisor {
	return &ContainerSupervisor{
		manager:       manager,
		logger:        logger,
		eventsChan:    make(chan DockerEvent, 100),
		stopChan:      make(chan struct{}),
		restartPolicy: policy,
		stats:         make(map[string]*ContainerStats),
	}
}

// Start begins monitoring container events
func (cs *ContainerSupervisor) Start(ctx context.Context) error {
	cs.runningMutex.Lock()
	defer cs.runningMutex.Unlock()

	if cs.running {
		return fmt.Errorf("supervisor is already running")
	}
	cs.running = true
	// Recreate stopChan in case it was closed before
	cs.stopChan = make(chan struct{})

	cs.logger.Info("starting container supervisor")

	// Start Docker events monitoring in background
	go cs.monitorDockerEvents(ctx)

	// Start event processor
	go cs.processEvents(ctx)

	// Start periodic state sync
	go cs.periodicStateSync(ctx)

	return nil
}

// Stop stops the container supervisor
func (cs *ContainerSupervisor) Stop() {
	cs.runningMutex.Lock()
	if !cs.running {
		cs.runningMutex.Unlock()
		return
	}
	cs.running = false
	cs.runningMutex.Unlock()

	cs.logger.Info("stopping container supervisor")
	close(cs.stopChan)
}

// GetContainerStats returns statistics for a container
func (cs *ContainerSupervisor) GetContainerStats(containerID string) (*ContainerStats, error) {
	cs.statsMutex.RLock()
	defer cs.statsMutex.RUnlock()

	stats, exists := cs.stats[containerID]
	if !exists {
		return nil, fmt.Errorf("no stats found for container %s", containerID)
	}

	// Return a copy to avoid race conditions
	stats.mu.RLock()
	statsCopy := &ContainerStats{
		RestartCount: stats.RestartCount,
		LastExitCode: stats.LastExitCode,
		Uptime:       stats.Uptime,
		LastRestart:  stats.LastRestart,
	}
	stats.mu.RUnlock()

	return statsCopy, nil
}

// monitorDockerEvents monitors Docker events stream
func (cs *ContainerSupervisor) monitorDockerEvents(ctx context.Context) {
	for {
		// Get stopChan safely
		cs.runningMutex.RLock()
		stopChan := cs.stopChan
		cs.runningMutex.RUnlock()

		select {
		case <-ctx.Done():
			return
		case <-stopChan:
			return
		default:
			cs.startEventStream(ctx)
			// If we get here, the event stream ended, wait before retry
			time.Sleep(5 * time.Second)
		}
	}
}

// startEventStream starts monitoring Docker events
func (cs *ContainerSupervisor) startEventStream(ctx context.Context) {
	cs.logger.Info("starting Docker events stream")

	// #nosec G204 - controlled docker execution is required for container orchestration
	cmd := exec.CommandContext(ctx, "docker", "events", "--format", "{{json .}}")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cs.logger.Error("failed to create stdout pipe for docker events", slog.String("error", err.Error()))
		return
	}

	if err := cmd.Start(); err != nil {
		cs.logger.Error("failed to start docker events", slog.String("error", err.Error()))
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var event DockerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			cs.logger.Warn("failed to parse docker event", slog.String("error", err.Error()))
			continue
		}

		// Filter for container events from our orchestrator
		if event.Type == "container" && cs.isOrchestratorContainer(event.Actor.Attributes["name"]) {
			// Get stopChan safely
			cs.runningMutex.RLock()
			stopChan := cs.stopChan
			cs.runningMutex.RUnlock()

			select {
			case cs.eventsChan <- event:
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			default:
				cs.logger.Warn("events channel full, dropping event")
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		cs.logger.Error("docker events command failed", slog.String("error", err.Error()))
	}
}

// processEvents processes Docker events and updates container states
func (cs *ContainerSupervisor) processEvents(ctx context.Context) {
	for {
		// Get stopChan safely
		cs.runningMutex.RLock()
		stopChan := cs.stopChan
		cs.runningMutex.RUnlock()

		select {
		case <-ctx.Done():
			return
		case <-stopChan:
			return
		case event := <-cs.eventsChan:
			cs.handleContainerEvent(ctx, event)
		}
	}
}

// handleContainerEvent processes a single container event
func (cs *ContainerSupervisor) handleContainerEvent(ctx context.Context, event DockerEvent) {
	containerName := event.Actor.Attributes["name"]

	cs.logger.Debug("handling container event",
		slog.String("container", containerName),
		slog.String("action", event.Action))

	// Find container by name
	containers, err := cs.manager.List()
	if err != nil {
		cs.logger.Error("failed to list containers", slog.String("error", err.Error()))
		return
	}

	var targetContainer *Container
	for _, c := range containers {
		if c.Name == containerName {
			targetContainer = c
			break
		}
	}

	if targetContainer == nil {
		cs.logger.Debug("container not found in manager", slog.String("name", containerName))
		return
	}

	switch event.Action {
	case "start":
		cs.handleContainerStart(targetContainer, event)
	case "die":
		cs.handleContainerDie(ctx, targetContainer, event)
	case "destroy":
		cs.handleContainerDestroy(targetContainer, event)
	}
}

// handleContainerStart handles container start events
func (cs *ContainerSupervisor) handleContainerStart(container *Container, event DockerEvent) {
	container.SetState(ContainerStateRunning)

	cs.logger.Info("container started",
		slog.String("id", container.ID),
		slog.String("name", container.Name))
}

// handleContainerDie handles container death events
func (cs *ContainerSupervisor) handleContainerDie(ctx context.Context, container *Container, event DockerEvent) {
	exitCodeStr := event.Actor.Attributes["exitCode"]

	cs.logger.Info("container died",
		slog.String("id", container.ID),
		slog.String("name", container.Name),
		slog.String("exit_code", exitCodeStr))

	// Update container state
	if exitCodeStr == "0" {
		container.SetState(ContainerStateStopped)
	} else {
		container.SetState(ContainerStateFailed)
	}

	// Update stats
	cs.updateContainerStats(container.ID, exitCodeStr)

	// Handle restart policy
	cs.handleRestartPolicy(ctx, container, exitCodeStr)
}

// handleContainerDestroy handles container destroy events
func (cs *ContainerSupervisor) handleContainerDestroy(container *Container, event DockerEvent) {
	cs.logger.Info("container destroyed",
		slog.String("id", container.ID),
		slog.String("name", container.Name))

	// Clean up stats
	cs.statsMutex.Lock()
	delete(cs.stats, container.ID)
	cs.statsMutex.Unlock()
}

// updateContainerStats updates statistics for a container
func (cs *ContainerSupervisor) updateContainerStats(containerID, exitCodeStr string) {
	cs.statsMutex.Lock()
	defer cs.statsMutex.Unlock()

	stats, exists := cs.stats[containerID]
	if !exists {
		stats = &ContainerStats{}
		cs.stats[containerID] = stats
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	// Parse exit code
	if exitCodeStr != "" && exitCodeStr != "0" {
		// Only count non-zero exit codes as failures
		if _, err := fmt.Sscanf(exitCodeStr, "%d", &stats.LastExitCode); err != nil {
			// If parsing fails, log and set to a default non-zero value
			cs.logger.Warn("failed to parse exit code",
				slog.String("exit_code", exitCodeStr),
				slog.String("error", err.Error()))
			stats.LastExitCode = 1 // Default failure exit code
		}
	}
}

// handleRestartPolicy implements restart policy logic
func (cs *ContainerSupervisor) handleRestartPolicy(ctx context.Context, container *Container, exitCodeStr string) {
	if cs.restartPolicy.Type == RestartPolicyNever {
		return
	}

	shouldRestart := false
	switch cs.restartPolicy.Type {
	case RestartPolicyAlways:
		shouldRestart = true
	case RestartPolicyOnFailure:
		shouldRestart = exitCodeStr != "0"
	}

	if !shouldRestart {
		return
	}

	// Check restart count limits
	cs.statsMutex.Lock()
	stats, exists := cs.stats[container.ID]
	if !exists {
		stats = &ContainerStats{}
		cs.stats[container.ID] = stats
	}

	stats.mu.Lock()
	currentRestarts := stats.RestartCount
	stats.mu.Unlock()
	cs.statsMutex.Unlock()

	if cs.restartPolicy.MaxRetries > 0 && currentRestarts >= cs.restartPolicy.MaxRetries {
		cs.logger.Warn("container restart limit reached",
			slog.String("id", container.ID),
			slog.Int("restarts", currentRestarts),
			slog.Int("max_retries", cs.restartPolicy.MaxRetries))
		return
	}

	// Schedule restart
	go cs.restartContainer(ctx, container)
}

// restartContainer restarts a container with backoff
func (cs *ContainerSupervisor) restartContainer(ctx context.Context, container *Container) {
	cs.statsMutex.Lock()
	stats, exists := cs.stats[container.ID]
	if !exists {
		stats = &ContainerStats{}
		cs.stats[container.ID] = stats
	}
	stats.mu.Lock()
	restartCount := stats.RestartCount
	stats.RestartCount++
	now := time.Now()
	stats.LastRestart = &now
	stats.mu.Unlock()
	cs.statsMutex.Unlock()

	// Exponential backoff: 1s, 2s, 4s, 8s, max 30s
	backoff := time.Duration(1<<restartCount) * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}

	cs.logger.Info("scheduling container restart",
		slog.String("id", container.ID),
		slog.Duration("delay", backoff),
		slog.Int("attempt", restartCount+1))

	container.SetState(ContainerStateRestarting)

	// Get stopChan safely
	cs.runningMutex.RLock()
	stopChan := cs.stopChan
	cs.runningMutex.RUnlock()

	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return
	case <-stopChan:
		return
	}

	// Remove the old container first to avoid name conflicts
	cs.cleanupContainerForRestart(ctx, container)

	if err := cs.manager.Start(ctx, container.ID); err != nil {
		cs.logger.Error("failed to restart container",
			slog.String("id", container.ID),
			slog.String("error", err.Error()))
		container.SetState(ContainerStateFailed)
	}
}

// cleanupContainerForRestart removes the Docker container to prepare for restart
func (cs *ContainerSupervisor) cleanupContainerForRestart(ctx context.Context, container *Container) {
	// Remove the old Docker container
	// #nosec G204 - controlled docker execution is required for container orchestration
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", container.Name)
	if err := cmd.Run(); err != nil {
		cs.logger.Debug("failed to cleanup container for restart",
			slog.String("id", container.ID),
			slog.String("error", err.Error()))
	}
}

// periodicStateSync periodically syncs container states with Docker
func (cs *ContainerSupervisor) periodicStateSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		// Get stopChan safely
		cs.runningMutex.RLock()
		stopChan := cs.stopChan
		cs.runningMutex.RUnlock()

		select {
		case <-ctx.Done():
			return
		case <-stopChan:
			return
		case <-ticker.C:
			cs.syncContainerStates(ctx)
		}
	}
}

// syncContainerStates syncs our container states with actual Docker states
func (cs *ContainerSupervisor) syncContainerStates(ctx context.Context) {
	containers, err := cs.manager.List()
	if err != nil {
		cs.logger.Error("failed to list containers for sync", slog.String("error", err.Error()))
		return
	}

	for _, container := range containers {
		cs.syncSingleContainerState(ctx, container)
	}
}

// syncSingleContainerState syncs a single container's state
func (cs *ContainerSupervisor) syncSingleContainerState(ctx context.Context, container *Container) {
	// #nosec G204 - controlled docker execution is required for container orchestration
	cmd := exec.CommandContext(ctx, "docker", "inspect", container.Name, "--format", "{{.State.Status}}")
	output, err := cmd.Output()
	if err != nil {
		// Container might not exist in Docker anymore
		if container.GetState() != ContainerStateStopped && container.GetState() != ContainerStateFailed {
			cs.logger.Warn("container not found in Docker, marking as failed",
				slog.String("id", container.ID),
				slog.String("name", container.Name))
			container.SetState(ContainerStateFailed)
		}
		return
	}

	dockerState := strings.TrimSpace(string(output))
	currentState := container.GetState()

	// Sync states if they differ
	switch dockerState {
	case "running":
		if currentState != ContainerStateRunning && currentState != ContainerStateRestarting {
			cs.logger.Info("syncing container state to running",
				slog.String("id", container.ID),
				slog.String("previous_state", string(currentState)))
			container.SetState(ContainerStateRunning)
		}
	case "exited":
		if currentState == ContainerStateRunning || currentState == ContainerStateRestarting {
			cs.logger.Info("syncing container state to stopped",
				slog.String("id", container.ID),
				slog.String("previous_state", string(currentState)))
			container.SetState(ContainerStateStopped)
		}
	}
}

// isOrchestratorContainer checks if a container belongs to our orchestrator
func (cs *ContainerSupervisor) isOrchestratorContainer(name string) bool {
	return strings.HasPrefix(name, "orch-")
}
