package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type DockerContainerManager struct{}

func NewDockerContainerManager() ContainerManager {
	return &DockerContainerManager{}
}

func (d *DockerContainerManager) CreateContainer(ctx context.Context, config ContainerConfig) (string, error) {
	if config.Image == "" {
		return "", fmt.Errorf("image name cannot be empty")
	}

	args := d.buildCreateArgs(config)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w, output: %s", err, string(output))
	}

	return d.extractContainerID(output)
}

func (d *DockerContainerManager) buildCreateArgs(config ContainerConfig) []string {
	args := []string{"create"}

	if config.Name != "" {
		args = append(args, "--name", config.Name)
	}

	args = d.appendResourceLimits(args, config.Resources)
	args = d.appendPortMappings(args, config.Ports)
	args = d.appendEnvironmentVars(args, config.Environment)
	args = d.appendVolumeMounts(args, config.Volumes)

	args = append(args, config.Image)

	if len(config.Command) > 0 {
		args = append(args, config.Command...)
	}

	return args
}

func (d *DockerContainerManager) appendResourceLimits(args []string, resources ResourceLimits) []string {
	if resources.CPUMillicores > 0 {
		cpuLimit := float64(resources.CPUMillicores) / 1000.0
		args = append(args, "--cpus", fmt.Sprintf("%.3f", cpuLimit))
	}

	if resources.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", resources.MemoryMB))
	}

	return args
}

func (d *DockerContainerManager) appendPortMappings(args []string, ports []PortMapping) []string {
	for _, port := range ports {
		portMapping := fmt.Sprintf("%d:%d", port.HostPort, port.ContainerPort)
		if port.Protocol != "" {
			portMapping += "/" + port.Protocol
		}
		args = append(args, "-p", portMapping)
	}
	return args
}

func (d *DockerContainerManager) appendEnvironmentVars(args []string, environment map[string]string) []string {
	for key, value := range environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}
	return args
}

func (d *DockerContainerManager) appendVolumeMounts(args []string, volumes []VolumeMount) []string {
	for _, volume := range volumes {
		volumeMount := fmt.Sprintf("%s:%s", volume.HostPath, volume.ContainerPath)
		if volume.ReadOnly {
			volumeMount += ":ro"
		}
		args = append(args, "-v", volumeMount)
	}
	return args
}

func (d *DockerContainerManager) extractContainerID(output []byte) (string, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	containerID := strings.TrimSpace(lines[len(lines)-1])

	if len(containerID) < 12 || !isHexString(containerID) {
		return "", fmt.Errorf("invalid container ID returned: %s", containerID)
	}

	return containerID, nil
}

func (d *DockerContainerManager) StartContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "start", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container %s: %w, output: %s", containerID, err, string(output))
	}
	return nil
}

func (d *DockerContainerManager) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	args := []string{"stop"}
	if timeout > 0 {
		args = append(args, "--time", strconv.Itoa(int(timeout.Seconds())))
	}
	args = append(args, containerID)

	cmd := exec.CommandContext(ctx, "docker", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}
	return nil
}

func (d *DockerContainerManager) RemoveContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", containerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}
	return nil
}

func (d *DockerContainerManager) GetContainerStatus(ctx context.Context, containerID string) (ContainerStatus, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		return ContainerStatus{}, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}

	return d.parseContainerStatus(containerID, output)
}

func (d *DockerContainerManager) parseContainerStatus(containerID string, output []byte) (ContainerStatus, error) {
	var inspectData []map[string]interface{}
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return ContainerStatus{}, fmt.Errorf("failed to parse inspect output: %w", err)
	}

	if len(inspectData) == 0 {
		return ContainerStatus{}, fmt.Errorf("container %s not found", containerID)
	}

	container := inspectData[0]
	state := container["State"].(map[string]interface{})

	status := ContainerStatus{
		ID:        containerID,
		Name:      strings.TrimPrefix(container["Name"].(string), "/"),
		State:     d.parseContainerState(state),
		Resources: ResourceUsage{Timestamp: time.Now()},
	}

	d.parseContainerTimes(&status, state)
	d.parseExitCode(&status, state)

	return status, nil
}

func (d *DockerContainerManager) parseContainerState(state map[string]interface{}) ContainerState {
	if running, ok := state["Running"].(bool); ok && running {
		return ContainerStateRunning
	}

	if statusStr, ok := state["Status"].(string); ok {
		switch statusStr {
		case "created":
			return ContainerStateCreated
		case "exited":
			return ContainerStateExited
		default:
			return ContainerStateStopped
		}
	}

	return ContainerStateStopped
}

func (d *DockerContainerManager) parseContainerTimes(status *ContainerStatus, state map[string]interface{}) {
	if startedAt, ok := state["StartedAt"].(string); ok && startedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, startedAt); err == nil {
			status.StartedAt = t
		}
	}

	if finishedAt, ok := state["FinishedAt"].(string); ok && finishedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, finishedAt); err == nil {
			status.FinishedAt = t
		}
	}
}

func (d *DockerContainerManager) parseExitCode(status *ContainerStatus, state map[string]interface{}) {
	if exitCode, ok := state["ExitCode"].(float64); ok {
		status.ExitCode = int(exitCode)
	}
}

func (d *DockerContainerManager) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return d.parseContainerList(output)
}

func (d *DockerContainerManager) parseContainerList(output []byte) ([]ContainerInfo, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var containers []ContainerInfo

	for _, line := range lines {
		if line == "" {
			continue
		}

		container, err := d.parseContainerLine(line)
		if err != nil {
			continue
		}

		containers = append(containers, container)
	}

	return containers, nil
}

func (d *DockerContainerManager) parseContainerLine(line string) (ContainerInfo, error) {
	parts := strings.Split(line, "\t")
	if len(parts) < 4 {
		return ContainerInfo{}, fmt.Errorf("invalid container line format")
	}

	container := ContainerInfo{
		ID:    parts[0],
		Name:  parts[1],
		Image: parts[2],
		State: d.parseContainerStateFromStatus(parts[3]),
	}

	if container.State == ContainerStateRunning {
		container.Uptime = d.parseUptimeFromStatus(parts[3])
	}

	return container, nil
}

func (d *DockerContainerManager) parseContainerStateFromStatus(status string) ContainerState {
	statusLower := strings.ToLower(status)

	if strings.Contains(statusLower, "up") {
		return ContainerStateRunning
	}

	if strings.Contains(statusLower, "exited") {
		return ContainerStateExited
	}

	return ContainerStateCreated
}

func (d *DockerContainerManager) parseUptimeFromStatus(status string) time.Duration {
	if strings.Contains(status, "minutes") || strings.Contains(status, "hours") || strings.Contains(status, "days") {
		return time.Minute
	}
	return 0
}

func isHexString(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
