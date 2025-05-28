package agent

import "time"

type ContainerConfig struct {
	Image       string            `json:"image"`
	Name        string            `json:"name"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Ports       []PortMapping     `json:"ports,omitempty"`
	Resources   ResourceLimits    `json:"resources,omitempty"`
	Volumes     []VolumeMount     `json:"volumes,omitempty"`
}

type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port,omitempty"`
	Protocol      string `json:"protocol"`
}

type ResourceLimits struct {
	CPUMillicores int64 `json:"cpu_millicores,omitempty"`
	MemoryMB      int64 `json:"memory_mb,omitempty"`
	DiskMB        int64 `json:"disk_mb,omitempty"`
}

type VolumeMount struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`
}

type ContainerStatus struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	State      ContainerState `json:"state"`
	ExitCode   int            `json:"exit_code,omitempty"`
	StartedAt  time.Time      `json:"started_at,omitempty"`
	FinishedAt time.Time      `json:"finished_at,omitempty"`
	Resources  ResourceUsage  `json:"resources"`
}

type ContainerState string

const (
	ContainerStateCreated ContainerState = "created"
	ContainerStateRunning ContainerState = "running"
	ContainerStateStopped ContainerState = "stopped"
	ContainerStateExited  ContainerState = "exited"
	ContainerStateError   ContainerState = "error"
)

type ContainerInfo struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Image  string         `json:"image"`
	State  ContainerState `json:"state"`
	Uptime time.Duration  `json:"uptime"`
}

type ResourceUsage struct {
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsedMB  int64     `json:"memory_used_mb"`
	MemoryTotalMB int64     `json:"memory_total_mb"`
	DiskUsedMB    int64     `json:"disk_used_mb"`
	DiskTotalMB   int64     `json:"disk_total_mb"`
	NetworkRxMB   int64     `json:"network_rx_mb"`
	NetworkTxMB   int64     `json:"network_tx_mb"`
	Timestamp     time.Time `json:"timestamp"`
}

type NodeStatus struct {
	NodeID     string          `json:"node_id"`
	Address    string          `json:"address"`
	Status     NodeState       `json:"status"`
	Resources  ResourceUsage   `json:"resources"`
	Containers []ContainerInfo `json:"containers"`
	LastSeen   time.Time       `json:"last_seen"`
	Version    string          `json:"version"`
}

type NodeState string

const (
	NodeStateHealthy   NodeState = "healthy"
	NodeStateUnhealthy NodeState = "unhealthy"
	NodeStateUnknown   NodeState = "unknown"
)

type ContainerRequest struct {
	Operation   ContainerOperation `json:"operation"`
	ContainerID string             `json:"container_id,omitempty"`
	Config      *ContainerConfig   `json:"config,omitempty"`
	Timeout     time.Duration      `json:"timeout,omitempty"`
}

type ContainerOperation string

const (
	ContainerOpCreate ContainerOperation = "create"
	ContainerOpStart  ContainerOperation = "start"
	ContainerOpStop   ContainerOperation = "stop"
	ContainerOpRemove ContainerOperation = "remove"
	ContainerOpStatus ContainerOperation = "status"
)

type ContainerResponse struct {
	Success     bool             `json:"success"`
	ContainerID string           `json:"container_id,omitempty"`
	Status      *ContainerStatus `json:"status,omitempty"`
	Error       string           `json:"error,omitempty"`
}
