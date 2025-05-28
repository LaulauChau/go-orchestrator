package agent

import (
	"context"
	"time"
)

type ContainerManager interface {
	CreateContainer(ctx context.Context, config ContainerConfig) (string, error)
	StartContainer(ctx context.Context, containerID string) error
	StopContainer(ctx context.Context, containerID string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, containerID string) error
	GetContainerStatus(ctx context.Context, containerID string) (ContainerStatus, error)
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
}

type ResourceMonitor interface {
	GetResourceUsage(ctx context.Context) (ResourceUsage, error)
	StartMonitoring(ctx context.Context, interval time.Duration) error
	StopMonitoring() error
}

type HeartbeatManager interface {
	StartHeartbeat(ctx context.Context, interval time.Duration) error
	StopHeartbeat() error
	SendHeartbeat(ctx context.Context, status NodeStatus) error
}

type NodeAgent interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HandleContainerRequest(ctx context.Context, req ContainerRequest) (ContainerResponse, error)
	GetNodeStatus(ctx context.Context) (NodeStatus, error)
}
