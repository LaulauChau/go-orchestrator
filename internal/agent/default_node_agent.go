package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultVersion                 = "v0.1.0"
	DefaultHeartbeatInterval       = 30 * time.Second
	DefaultResourceMonitorInterval = 10 * time.Second
)

type DefaultNodeAgent struct {
	nodeID          string
	address         string
	version         string
	controlPlaneURL string

	containerManager ContainerManager
	resourceMonitor  ResourceMonitor
	heartbeatManager HeartbeatManager

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
}

func NewDefaultNodeAgent(nodeID, address, controlPlaneURL string) *DefaultNodeAgent {
	return &DefaultNodeAgent{
		nodeID:           nodeID,
		address:          address,
		version:          DefaultVersion,
		controlPlaneURL:  controlPlaneURL,
		containerManager: NewDockerContainerManager(),
		resourceMonitor:  NewSystemResourceMonitor(),
		heartbeatManager: NewHTTPHeartbeatManager(controlPlaneURL),
	}
}

func (n *DefaultNodeAgent) Start(ctx context.Context) error {
	if n.nodeID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}
	if n.address == "" {
		return fmt.Errorf("node address cannot be empty")
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.running {
		return fmt.Errorf("node agent is already running")
	}

	// Start resource monitoring
	err := n.resourceMonitor.StartMonitoring(ctx, DefaultResourceMonitorInterval)
	if err != nil {
		return fmt.Errorf("failed to start resource monitor: %w", err)
	}

	// Start heartbeat
	err = n.heartbeatManager.StartHeartbeat(ctx, DefaultHeartbeatInterval)
	if err != nil {
		// Stop resource monitor if heartbeat fails
		_ = n.resourceMonitor.StopMonitoring()
		return fmt.Errorf("failed to start heartbeat: %w", err)
	}

	n.running = true
	n.stopCh = make(chan struct{})

	// Start heartbeat sending loop
	go n.heartbeatLoop(ctx)

	return nil
}

func (n *DefaultNodeAgent) Stop(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.running {
		return fmt.Errorf("node agent is not running")
	}

	n.running = false
	close(n.stopCh)

	// Stop all components
	var errs []error

	if err := n.heartbeatManager.StopHeartbeat(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop heartbeat: %w", err))
	}

	if err := n.resourceMonitor.StopMonitoring(); err != nil {
		errs = append(errs, fmt.Errorf("failed to stop resource monitor: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors stopping components: %v", errs)
	}

	return nil
}

func (n *DefaultNodeAgent) HandleContainerRequest(ctx context.Context, req ContainerRequest) (ContainerResponse, error) {
	n.mu.RLock()
	if !n.running {
		n.mu.RUnlock()
		return ContainerResponse{
			Success: false,
			Error:   "node agent is not running",
		}, fmt.Errorf("node agent is not running")
	}
	n.mu.RUnlock()

	switch req.Operation {
	case ContainerOpCreate:
		return n.handleCreateContainer(ctx, req)
	case ContainerOpStart:
		return n.handleStartContainer(ctx, req)
	case ContainerOpStop:
		return n.handleStopContainer(ctx, req)
	case ContainerOpRemove:
		return n.handleRemoveContainer(ctx, req)
	case ContainerOpStatus:
		return n.handleGetContainerStatus(ctx, req)
	default:
		return ContainerResponse{
			Success: false,
			Error:   fmt.Sprintf("unsupported operation: %s", req.Operation),
		}, fmt.Errorf("unsupported operation: %s", req.Operation)
	}
}

func (n *DefaultNodeAgent) GetNodeStatus(ctx context.Context) (NodeStatus, error) {
	n.mu.RLock()
	if !n.running {
		n.mu.RUnlock()
		return NodeStatus{}, fmt.Errorf("node agent is not running")
	}
	n.mu.RUnlock()

	// Get current resource usage
	resources, err := n.resourceMonitor.GetResourceUsage(ctx)
	if err != nil {
		return NodeStatus{}, fmt.Errorf("failed to get resource usage: %w", err)
	}

	// Get container list
	containers, err := n.containerManager.ListContainers(ctx)
	if err != nil {
		return NodeStatus{}, fmt.Errorf("failed to list containers: %w", err)
	}

	return NodeStatus{
		NodeID:     n.nodeID,
		Address:    n.address,
		Status:     NodeStateHealthy,
		Resources:  resources,
		Containers: containers,
		LastSeen:   time.Now(),
		Version:    n.version,
	}, nil
}

func (n *DefaultNodeAgent) handleCreateContainer(ctx context.Context, req ContainerRequest) (ContainerResponse, error) {
	if req.Config == nil {
		return ContainerResponse{
			Success: false,
			Error:   "container config is required for create operation",
		}, fmt.Errorf("container config is required")
	}

	containerID, err := n.containerManager.CreateContainer(ctx, *req.Config)
	if err != nil {
		return ContainerResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return ContainerResponse{
		Success:     true,
		ContainerID: containerID,
	}, nil
}

func (n *DefaultNodeAgent) handleStartContainer(ctx context.Context, req ContainerRequest) (ContainerResponse, error) {
	if req.ContainerID == "" {
		return ContainerResponse{
			Success: false,
			Error:   "container ID is required for start operation",
		}, fmt.Errorf("container ID is required")
	}

	err := n.containerManager.StartContainer(ctx, req.ContainerID)
	if err != nil {
		return ContainerResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return ContainerResponse{
		Success:     true,
		ContainerID: req.ContainerID,
	}, nil
}

func (n *DefaultNodeAgent) handleStopContainer(ctx context.Context, req ContainerRequest) (ContainerResponse, error) {
	if req.ContainerID == "" {
		return ContainerResponse{
			Success: false,
			Error:   "container ID is required for stop operation",
		}, fmt.Errorf("container ID is required")
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second // Default timeout
	}

	err := n.containerManager.StopContainer(ctx, req.ContainerID, timeout)
	if err != nil {
		return ContainerResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return ContainerResponse{
		Success:     true,
		ContainerID: req.ContainerID,
	}, nil
}

func (n *DefaultNodeAgent) handleRemoveContainer(ctx context.Context, req ContainerRequest) (ContainerResponse, error) {
	if req.ContainerID == "" {
		return ContainerResponse{
			Success: false,
			Error:   "container ID is required for remove operation",
		}, fmt.Errorf("container ID is required")
	}

	err := n.containerManager.RemoveContainer(ctx, req.ContainerID)
	if err != nil {
		return ContainerResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return ContainerResponse{
		Success:     true,
		ContainerID: req.ContainerID,
	}, nil
}

func (n *DefaultNodeAgent) handleGetContainerStatus(ctx context.Context, req ContainerRequest) (ContainerResponse, error) {
	if req.ContainerID == "" {
		return ContainerResponse{
			Success: false,
			Error:   "container ID is required for status operation",
		}, fmt.Errorf("container ID is required")
	}

	status, err := n.containerManager.GetContainerStatus(ctx, req.ContainerID)
	if err != nil {
		return ContainerResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	return ContainerResponse{
		Success: true,
		Status:  &status,
	}, nil
}

func (n *DefaultNodeAgent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(DefaultHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-n.stopCh:
			return
		case <-ticker.C:
			// Get current node status and send heartbeat
			status, err := n.GetNodeStatus(ctx)
			if err != nil {
				// Log error but continue (in a real implementation, we'd use a logger)
				continue
			}

			err = n.heartbeatManager.SendHeartbeat(ctx, status)
			if err != nil {
				// Log error but continue (in a real implementation, we'd use a logger)
				continue
			}
		}
	}
}
