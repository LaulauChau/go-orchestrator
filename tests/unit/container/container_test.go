package container_test

import (
	"testing"
	"time"

	"github.com/lachau/go-orchestrator/internal/container"
)

func TestContainer_IsRunning(t *testing.T) {
	tests := []struct {
		name  string
		state container.ContainerState
		want  bool
	}{
		{"running container", container.ContainerStateRunning, true},
		{"created container", container.ContainerStateCreated, false},
		{"stopped container", container.ContainerStateStopped, false},
		{"failed container", container.ContainerStateFailed, false},
		{"restarting container", container.ContainerStateRestarting, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &container.Container{}
			c.SetStateForTest(tt.state)
			if got := c.IsRunning(); got != tt.want {
				t.Errorf("Container.IsRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainer_IsStopped(t *testing.T) {
	tests := []struct {
		name  string
		state container.ContainerState
		want  bool
	}{
		{"running container", container.ContainerStateRunning, false},
		{"created container", container.ContainerStateCreated, false},
		{"stopped container", container.ContainerStateStopped, true},
		{"failed container", container.ContainerStateFailed, true},
		{"restarting container", container.ContainerStateRestarting, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &container.Container{}
			c.SetStateForTest(tt.state)
			if got := c.IsStopped(); got != tt.want {
				t.Errorf("Container.IsStopped() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainer_SetState(t *testing.T) {
	t.Run("sets running state with timestamp", func(t *testing.T) {
		c := &container.Container{}
		c.SetStateForTest(container.ContainerStateCreated)
		before := time.Now()

		c.SetState(container.ContainerStateRunning)

		if c.GetState() != container.ContainerStateRunning {
			t.Errorf("Expected state %v, got %v", container.ContainerStateRunning, c.GetState())
		}

		if c.GetStarted() == nil {
			t.Error("Expected Started timestamp to be set")
		}

		if c.GetStarted().Before(before) {
			t.Error("Started timestamp should be after test start")
		}
	})

	t.Run("sets stopped state with timestamp", func(t *testing.T) {
		c := &container.Container{}
		c.SetStateForTest(container.ContainerStateRunning)
		before := time.Now()

		c.SetState(container.ContainerStateStopped)

		if c.GetState() != container.ContainerStateStopped {
			t.Errorf("Expected state %v, got %v", container.ContainerStateStopped, c.GetState())
		}

		if c.GetFinished() == nil {
			t.Error("Expected Finished timestamp to be set")
		}

		if c.GetFinished().Before(before) {
			t.Error("Finished timestamp should be after test start")
		}
	})

	t.Run("sets failed state with timestamp", func(t *testing.T) {
		c := &container.Container{}
		c.SetStateForTest(container.ContainerStateRunning)
		before := time.Now()

		c.SetState(container.ContainerStateFailed)

		if c.GetState() != container.ContainerStateFailed {
			t.Errorf("Expected state %v, got %v", container.ContainerStateFailed, c.GetState())
		}

		if c.GetFinished() == nil {
			t.Error("Expected Finished timestamp to be set")
		}

		if c.GetFinished().Before(before) {
			t.Error("Finished timestamp should be after test start")
		}
	})

	t.Run("does not overwrite existing timestamps", func(t *testing.T) {
		startTime := time.Now().Add(-time.Hour)
		c := &container.Container{}
		c.SetStateForTest(container.ContainerStateRunning)
		// We need to set Started manually for this test since SetStateForTest doesn't set timestamps
		c.SetStartedForTest(&startTime)

		c.SetState(container.ContainerStateRunning)

		if c.GetStarted().Equal(startTime) == false {
			t.Error("Started timestamp should not be overwritten")
		}
	})
}
