package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LaulauChau/go-orchestrator/internal/agent"
)

func main() {
	fmt.Println("🔄 HeartbeatManager Demo")
	fmt.Println("========================")

	// Create HeartbeatManager
	controlPlaneURL := "http://localhost:8080"
	heartbeatManager := agent.NewHTTPHeartbeatManager(controlPlaneURL)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start heartbeat with 5-second interval
	fmt.Printf("📡 Starting heartbeat to %s every 5 seconds...\n", controlPlaneURL)
	err := heartbeatManager.StartHeartbeat(ctx, 5*time.Second)
	if err != nil {
		log.Fatalf("Failed to start heartbeat: %v", err)
	}

	fmt.Printf("✅ Heartbeat started successfully\n")
	fmt.Printf("🔍 Status: Running = %v\n\n", heartbeatManager.IsRunning())

	// Create sample node status
	nodeStatus := agent.NodeStatus{
		NodeID:  "demo-node-001",
		Address: "192.168.1.100:8081",
		Status:  agent.NodeStateHealthy,
		Resources: agent.ResourceUsage{
			CPUPercent:    15.5,
			MemoryUsedMB:  2048,
			MemoryTotalMB: 8192,
			DiskUsedMB:    45000,
			DiskTotalMB:   100000,
			NetworkRxMB:   150,
			NetworkTxMB:   75,
			Timestamp:     time.Now(),
		},
		Containers: []agent.ContainerInfo{
			{
				ID:     "container-001",
				Name:   "web-server",
				Image:  "nginx:latest",
				State:  agent.ContainerStateRunning,
				Uptime: 2 * time.Hour,
			},
			{
				ID:     "container-002",
				Name:   "database",
				Image:  "postgres:13",
				State:  agent.ContainerStateRunning,
				Uptime: 1 * time.Hour,
			},
		},
		LastSeen: time.Now(),
		Version:  "v0.1.0",
	}

	// Send periodic heartbeats manually to demonstrate functionality
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	fmt.Println("📊 Sending heartbeat status updates...")
	fmt.Println("   (Note: Connection errors are expected since no control plane is running)")
	fmt.Println()

	heartbeatCount := 0

	for {
		select {
		case <-sigCh:
			fmt.Println("\n🛑 Received shutdown signal...")
			goto shutdown

		case <-ticker.C:
			heartbeatCount++

			// Update timestamp and some dynamic values
			nodeStatus.LastSeen = time.Now()
			nodeStatus.Resources.Timestamp = time.Now()
			nodeStatus.Resources.CPUPercent = 10.0 + float64(heartbeatCount%20) // Simulate varying CPU
			nodeStatus.Resources.NetworkRxMB += int64(heartbeatCount * 5)       // Simulate network activity
			nodeStatus.Resources.NetworkTxMB += int64(heartbeatCount * 2)

			fmt.Printf("💓 Heartbeat #%d - %s\n", heartbeatCount, time.Now().Format("15:04:05"))
			fmt.Printf("   Node: %s (%s)\n", nodeStatus.NodeID, nodeStatus.Status)
			fmt.Printf("   CPU: %.1f%%, Memory: %d/%d MB\n",
				nodeStatus.Resources.CPUPercent,
				nodeStatus.Resources.MemoryUsedMB,
				nodeStatus.Resources.MemoryTotalMB)
			fmt.Printf("   Containers: %d running\n", len(nodeStatus.Containers))

			// Send heartbeat
			err := heartbeatManager.SendHeartbeat(ctx, nodeStatus)
			if err != nil {
				fmt.Printf("   ❌ Error: %v\n", err)
			} else {
				fmt.Printf("   ✅ Heartbeat sent successfully\n")
			}
			fmt.Println()

			// Stop after 5 heartbeats for demo purposes
			if heartbeatCount >= 5 {
				fmt.Println("📝 Demo completed after 5 heartbeats")
				goto shutdown
			}

		case <-ctx.Done():
			fmt.Println("\n🔄 Context cancelled...")
			goto shutdown
		}
	}

shutdown:
	fmt.Println("🔄 Stopping heartbeat manager...")

	err = heartbeatManager.StopHeartbeat()
	if err != nil {
		fmt.Printf("❌ Error stopping heartbeat: %v\n", err)
	} else {
		fmt.Printf("✅ Heartbeat stopped successfully\n")
	}

	fmt.Printf("🔍 Final status: Running = %v\n", heartbeatManager.IsRunning())
	fmt.Println("👋 Demo completed!")
}
