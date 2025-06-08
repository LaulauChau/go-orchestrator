package container

import (
	"time"
)

// ContainerState represents the current state of a container
type ContainerState string

const (
	ContainerStateCreated    ContainerState = "created"
	ContainerStateRunning    ContainerState = "running"
	ContainerStateStopped    ContainerState = "stopped"
	ContainerStateFailed     ContainerState = "failed"
	ContainerStateRestarting ContainerState = "restarting"
)

// Container represents a container instance
type Container struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Image    string          `json:"image"`
	State    ContainerState  `json:"state"`
	Config   ContainerConfig `json:"config"`
	Created  time.Time       `json:"created"`
	Started  *time.Time      `json:"started,omitempty"`
	Finished *time.Time      `json:"finished,omitempty"`
}

// ContainerConfig holds configuration for a container
type ContainerConfig struct {
	Image      string            `json:"image"`
	Command    []string          `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Ports      []PortMapping     `json:"ports,omitempty"`
	Volumes    []VolumeMount     `json:"volumes,omitempty"`
}

// PortMapping represents a port mapping configuration
type PortMapping struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"` // tcp, udp
}

// VolumeMount represents a volume mount configuration
type VolumeMount struct {
	Source      string `json:"source"`      // Host path
	Destination string `json:"destination"` // Container path
	ReadOnly    bool   `json:"read_only"`
}

// IsRunning returns true if the container is in running state
func (c *Container) IsRunning() bool {
	return c.State == ContainerStateRunning
}

// IsStopped returns true if the container is stopped or failed
func (c *Container) IsStopped() bool {
	return c.State == ContainerStateStopped || c.State == ContainerStateFailed
}

// SetState updates the container state and timestamps
func (c *Container) SetState(state ContainerState) {
	c.State = state
	now := time.Now()

	switch state {
	case ContainerStateRunning:
		if c.Started == nil {
			c.Started = &now
		}
	case ContainerStateStopped, ContainerStateFailed:
		if c.Finished == nil {
			c.Finished = &now
		}
	}
}
