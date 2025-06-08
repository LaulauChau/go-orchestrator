package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lachau/go-orchestrator/internal/container"
)

var (
	logger *slog.Logger
	cm     container.ContainerManager
)

func main() {
	// Set up logger
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Set up graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Create container manager
	cm = container.NewDockerManager(logger)

	// Execute root command
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		logger.Error("command execution failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "go-orchestrator",
	Short: "A container orchestrator built in Go",
	Long: `go-orchestrator is a container orchestration platform that provides 
container lifecycle management, scheduling, and resource control.`,
}

var runCmd = &cobra.Command{
	Use:   "run [image]",
	Short: "Create and start a container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		image := args[0]

		config := container.ContainerConfig{
			Image: image,
		}

		// Create container
		c, err := cm.Create(cmd.Context(), config)
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}

		// Start container
		err = cm.Start(cmd.Context(), c.ID)
		if err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}

		fmt.Printf("Container started: %s\n", c.ID)
		fmt.Printf("Name: %s\n", c.Name)
		fmt.Printf("Image: %s\n", c.Image)
		return nil
	},
}

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List all containers",
	RunE: func(cmd *cobra.Command, args []string) error {
		containers, err := cm.List()
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}

		if len(containers) == 0 {
			fmt.Println("No containers found")
			return nil
		}

		fmt.Printf("%-12s %-20s %-20s %-10s %-20s\n", "ID", "NAME", "IMAGE", "STATE", "CREATED")
		fmt.Println("------------------------------------------------------------")

		for _, c := range containers {
			shortID := c.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}

			name := c.Name
			if len(name) > 20 {
				name = name[:17] + "..."
			}

			image := c.Image
			if len(image) > 20 {
				image = image[:17] + "..."
			}

			fmt.Printf("%-12s %-20s %-20s %-10s %-20s\n",
				shortID, name, image, string(c.State), c.Created.Format("2006-01-02 15:04:05"))
		}
		return nil
	},
}

var startCmd = &cobra.Command{
	Use:   "start [container-id]",
	Short: "Start a container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		containerID := args[0]

		err := cm.Start(cmd.Context(), containerID)
		if err != nil {
			return fmt.Errorf("failed to start container %s: %w", containerID, err)
		}

		fmt.Printf("Container started: %s\n", containerID)
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop [container-id]",
	Short: "Stop a container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		containerID := args[0]

		err := cm.Stop(cmd.Context(), containerID)
		if err != nil {
			return fmt.Errorf("failed to stop container %s: %w", containerID, err)
		}

		fmt.Printf("Container stopped: %s\n", containerID)
		return nil
	},
}

var rmCmd = &cobra.Command{
	Use:   "rm [container-id]",
	Short: "Remove a container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		containerID := args[0]

		err := cm.Delete(cmd.Context(), containerID)
		if err != nil {
			return fmt.Errorf("failed to delete container %s: %w", containerID, err)
		}

		fmt.Printf("Container deleted: %s\n", containerID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(rmCmd)
}
