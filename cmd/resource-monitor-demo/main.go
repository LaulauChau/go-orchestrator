package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/LaulauChau/go-orchestrator/internal/agent"
)

func main() {
	fmt.Println("🔍 ResourceMonitor Demo")
	fmt.Println("========================")

	monitor := agent.NewSystemResourceMonitor()

	ctx := context.Background()

	fmt.Println("\n📊 Current System Resources:")
	usage, err := monitor.GetResourceUsage(ctx)
	if err != nil {
		log.Fatalf("Failed to get resource usage: %v", err)
	}

	printResourceUsage(usage)

	fmt.Println("\n🔄 Starting continuous monitoring (5 samples)...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = monitor.StartMonitoring(ctx, 2*time.Second)
	if err != nil {
		log.Fatalf("Failed to start monitoring: %v", err)
	}

	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Second)

		usage, err := monitor.GetResourceUsage(ctx)
		if err != nil {
			log.Printf("Failed to get resource usage: %v", err)
			continue
		}

		fmt.Printf("\n📈 Sample %d:\n", i+1)
		printResourceUsage(usage)
	}

	err = monitor.StopMonitoring()
	if err != nil {
		log.Printf("Failed to stop monitoring: %v", err)
	}

	fmt.Println("\n✅ Demo completed!")
}

func printResourceUsage(usage agent.ResourceUsage) {
	fmt.Printf("  CPU Usage:    %.2f%%\n", usage.CPUPercent)
	fmt.Printf("  Memory:       %d MB / %d MB (%.1f%%)\n",
		usage.MemoryUsedMB, usage.MemoryTotalMB,
		float64(usage.MemoryUsedMB)/float64(usage.MemoryTotalMB)*100)
	fmt.Printf("  Disk:         %d MB / %d MB (%.1f%%)\n",
		usage.DiskUsedMB, usage.DiskTotalMB,
		float64(usage.DiskUsedMB)/float64(usage.DiskTotalMB)*100)
	fmt.Printf("  Network:      RX: %d MB, TX: %d MB\n",
		usage.NetworkRxMB, usage.NetworkTxMB)
	fmt.Printf("  Timestamp:    %s\n", usage.Timestamp.Format("15:04:05"))
}
