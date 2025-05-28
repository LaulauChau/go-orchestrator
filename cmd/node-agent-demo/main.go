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
	fmt.Println("🚀 Container Orchestrator - Node Agent Demo")
	fmt.Println("==========================================")

	// Create a new node agent
	nodeID := "demo-node-1"
	address := "192.168.1.100:8081"
	controlPlaneURL := "http://localhost:8080"

	nodeAgent := agent.NewDefaultNodeAgent(nodeID, address, controlPlaneURL)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start the node agent
	fmt.Printf("📡 Starting Node Agent (ID: %s, Address: %s)\n", nodeID, address)
	err := nodeAgent.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start node agent: %v", err)
	}
	fmt.Println("✅ Node Agent started successfully")

	// Demonstrate getting node status
	fmt.Println("\n📊 Getting Node Status...")
	status, err := nodeAgent.GetNodeStatus(ctx)
	if err != nil {
		log.Printf("Failed to get node status: %v", err)
	} else {
		fmt.Printf("   Node ID: %s\n", status.NodeID)
		fmt.Printf("   Address: %s\n", status.Address)
		fmt.Printf("   Status: %s\n", status.Status)
		fmt.Printf("   Version: %s\n", status.Version)
		fmt.Printf("   CPU Usage: %.2f%%\n", status.Resources.CPUPercent)
		fmt.Printf("   Memory: %d/%d MB\n", status.Resources.MemoryUsedMB, status.Resources.MemoryTotalMB)
		fmt.Printf("   Containers: %d\n", len(status.Containers))
	}

	// Demonstrate container operations
	fmt.Println("\n🐳 Demonstrating Container Operations...")

	// Create a container
	fmt.Println("   Creating nginx container...")
	createReq := agent.ContainerRequest{
		Operation: agent.ContainerOpCreate,
		Config: &agent.ContainerConfig{
			Image: "nginx:latest",
			Name:  "demo-nginx-" + time.Now().Format("20060102-150405"),
			Ports: []agent.PortMapping{
				{
					ContainerPort: 80,
					HostPort:      8080,
					Protocol:      "tcp",
				},
			},
			Environment: map[string]string{
				"NGINX_HOST": "localhost",
				"NGINX_PORT": "80",
			},
		},
	}

	createResp, err := nodeAgent.HandleContainerRequest(ctx, createReq)
	if err != nil {
		log.Printf("Failed to create container: %v", err)
	} else if createResp.Success {
		fmt.Printf("   ✅ Container created: %s\n", createResp.ContainerID)

		// Start the container
		fmt.Println("   Starting container...")
		startReq := agent.ContainerRequest{
			Operation:   agent.ContainerOpStart,
			ContainerID: createResp.ContainerID,
		}

		startResp, err := nodeAgent.HandleContainerRequest(ctx, startReq)
		if err != nil {
			log.Printf("Failed to start container: %v", err)
		} else if startResp.Success {
			fmt.Printf("   ✅ Container started: %s\n", startResp.ContainerID)

			// Get container status
			fmt.Println("   Getting container status...")
			statusReq := agent.ContainerRequest{
				Operation:   agent.ContainerOpStatus,
				ContainerID: createResp.ContainerID,
			}

			statusResp, err := nodeAgent.HandleContainerRequest(ctx, statusReq)
			if err != nil {
				log.Printf("Failed to get container status: %v", err)
			} else if statusResp.Success && statusResp.Status != nil {
				fmt.Printf("   📊 Container Status:\n")
				fmt.Printf("      ID: %s\n", statusResp.Status.ID)
				fmt.Printf("      Name: %s\n", statusResp.Status.Name)
				fmt.Printf("      State: %s\n", statusResp.Status.State)
				if !statusResp.Status.StartedAt.IsZero() {
					fmt.Printf("      Started: %s\n", statusResp.Status.StartedAt.Format(time.RFC3339))
				}
			}

			// Wait a bit to let the container run
			fmt.Println("   Letting container run for 5 seconds...")
			time.Sleep(5 * time.Second)

			// Stop the container
			fmt.Println("   Stopping container...")
			stopReq := agent.ContainerRequest{
				Operation:   agent.ContainerOpStop,
				ContainerID: createResp.ContainerID,
				Timeout:     10 * time.Second,
			}

			stopResp, err := nodeAgent.HandleContainerRequest(ctx, stopReq)
			if err != nil {
				log.Printf("Failed to stop container: %v", err)
			} else if stopResp.Success {
				fmt.Printf("   ✅ Container stopped: %s\n", stopResp.ContainerID)
			}

			// Remove the container
			fmt.Println("   Removing container...")
			removeReq := agent.ContainerRequest{
				Operation:   agent.ContainerOpRemove,
				ContainerID: createResp.ContainerID,
			}

			removeResp, err := nodeAgent.HandleContainerRequest(ctx, removeReq)
			if err != nil {
				log.Printf("Failed to remove container: %v", err)
			} else if removeResp.Success {
				fmt.Printf("   ✅ Container removed: %s\n", removeResp.ContainerID)
			}
		}
	}

	// Show updated node status
	fmt.Println("\n📊 Final Node Status...")
	finalStatus, err := nodeAgent.GetNodeStatus(ctx)
	if err != nil {
		log.Printf("Failed to get final node status: %v", err)
	} else {
		fmt.Printf("   Active Containers: %d\n", len(finalStatus.Containers))
		fmt.Printf("   CPU Usage: %.2f%%\n", finalStatus.Resources.CPUPercent)
		fmt.Printf("   Memory: %d/%d MB\n", finalStatus.Resources.MemoryUsedMB, finalStatus.Resources.MemoryTotalMB)
	}

	// Wait for signal or timeout
	fmt.Println("\n⏳ Node Agent is running. Press Ctrl+C to stop or wait 30 seconds...")

	select {
	case <-sigCh:
		fmt.Println("\n🛑 Received shutdown signal")
	case <-time.After(30 * time.Second):
		fmt.Println("\n⏰ Demo timeout reached")
	}

	// Graceful shutdown
	fmt.Println("🔄 Shutting down Node Agent...")
	err = nodeAgent.Stop(ctx)
	if err != nil {
		log.Printf("Error stopping node agent: %v", err)
	} else {
		fmt.Println("✅ Node Agent stopped successfully")
	}

	fmt.Println("👋 Demo completed!")
}
