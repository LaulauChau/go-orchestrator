package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type HTTPHeartbeatManager struct {
	controlPlaneURL string
	client          *http.Client
	mu              sync.RWMutex
	running         bool
	stopCh          chan struct{}
	ticker          *time.Ticker
}

func NewHTTPHeartbeatManager(controlPlaneURL string) *HTTPHeartbeatManager {
	return &HTTPHeartbeatManager{
		controlPlaneURL: controlPlaneURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (h *HTTPHeartbeatManager) StartHeartbeat(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive, got %v", interval)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return fmt.Errorf("heartbeat is already running")
	}

	h.running = true
	h.stopCh = make(chan struct{})
	h.ticker = time.NewTicker(interval)

	go h.heartbeatLoop(ctx)

	return nil
}

func (h *HTTPHeartbeatManager) StopHeartbeat() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running {
		return fmt.Errorf("heartbeat is not running")
	}

	h.running = false
	if h.ticker != nil {
		h.ticker.Stop()
	}
	close(h.stopCh)

	return nil
}

func (h *HTTPHeartbeatManager) SendHeartbeat(ctx context.Context, status NodeStatus) error {
	payload, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal node status: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/nodes/%s/heartbeat", h.controlPlaneURL, status.NodeID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create heartbeat request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("heartbeat request failed with status %d", resp.StatusCode)
	}

	return nil
}

func (h *HTTPHeartbeatManager) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

func (h *HTTPHeartbeatManager) heartbeatLoop(ctx context.Context) {
	defer func() {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-h.ticker.C:
			// In a real implementation, this would get the current node status
			// and send it. For now, we just continue the loop.
			// The actual heartbeat sending will be triggered by the NodeAgent
			continue
		}
	}
}
