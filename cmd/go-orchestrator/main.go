package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/lachau/go-orchestrator/internal/container"
)

var (
	logger     *slog.Logger
	cm         container.ContainerManager
	supervisor *container.ContainerSupervisor
	setupOnce  sync.Once
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "go-orchestrator",
	Short: "A container orchestrator built in Go",
	Long: `go-orchestrator is a container orchestration platform that provides 
container lifecycle management, scheduling, and resource control.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Start supervisor for all commands except help
		if cmd.Name() != "help" && cmd.Name() != "completion" {
			ctx := context.Background()
			if err := supervisor.Start(ctx); err != nil {
				logger.Error("failed to start supervisor", slog.String("error", err.Error()))
			}

			// Handle graceful shutdown (only start once)
			setupOnce.Do(func() {
				go func() {
					sigChan := make(chan os.Signal, 1)
					signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
					<-sigChan
					logger.Info("shutting down gracefully...")
					supervisor.Stop()
					os.Exit(0)
				}()
			})
		}
	},
}

var runCmd = &cobra.Command{
	Use:   "run [image] [command] [args...]",
	Short: "Create and start a container",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		image := args[0]

		config := container.ContainerConfig{
			Image: image,
		}

		// Add command and args if provided
		if len(args) > 1 {
			config.Command = []string{args[1]}
			if len(args) > 2 {
				config.Args = args[2:]
			}
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
				shortID, name, image, string(c.GetState()), c.Created.Format("2006-01-02 15:04:05"))
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

var statsCmd = &cobra.Command{
	Use:   "stats [container-id]",
	Short: "Show container statistics",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return showAllStats()
		}
		return showContainerStats(args[0])
	},
}

func showAllStats() error {
	containers, err := cm.List()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "CONTAINER ID\tNAME\tSTATE\tRESTARTS\tLAST EXIT\tUPTIME"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	for _, c := range containers {
		stats, err := supervisor.GetContainerStats(c.ID)
		if err != nil {
			// Container might not have stats yet
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
				c.ID[:12], c.Name, c.GetState(), 0, "-", "-"); err != nil {
				return fmt.Errorf("failed to write container row: %w", err)
			}
			continue
		}

		uptime := "-"
		if c.GetStarted() != nil && c.GetState() == container.ContainerStateRunning {
			uptime = time.Since(*c.GetStarted()).Truncate(time.Second).String()
		}

		lastExit := "-"
		if stats.LastExitCode != 0 {
			lastExit = fmt.Sprintf("%d", stats.LastExitCode)
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			c.ID[:12], c.Name, c.GetState(), stats.RestartCount, lastExit, uptime); err != nil {
			return fmt.Errorf("failed to write container row: %w", err)
		}
	}

	return w.Flush()
}

func showContainerStats(containerID string) error {
	c, err := cm.Get(containerID)
	if err != nil {
		return fmt.Errorf("failed to get container: %w", err)
	}

	stats, err := supervisor.GetContainerStats(containerID)
	if err != nil {
		return fmt.Errorf("failed to get container stats: %w", err)
	}

	fmt.Printf("Container: %s\n", c.ID)
	fmt.Printf("Name: %s\n", c.Name)
	fmt.Printf("Image: %s\n", c.Image)
	fmt.Printf("State: %s\n", c.GetState())
	fmt.Printf("Created: %s\n", c.Created.Format(time.RFC3339))

	if c.GetStarted() != nil {
		fmt.Printf("Started: %s\n", c.GetStarted().Format(time.RFC3339))
		if c.GetState() == container.ContainerStateRunning {
			fmt.Printf("Uptime: %s\n", time.Since(*c.GetStarted()).Truncate(time.Second))
		}
	}

	if c.GetFinished() != nil {
		fmt.Printf("Finished: %s\n", c.GetFinished().Format(time.RFC3339))
	}

	fmt.Printf("Restart Count: %d\n", stats.RestartCount)

	if stats.LastExitCode != 0 {
		fmt.Printf("Last Exit Code: %d\n", stats.LastExitCode)
	}

	if stats.LastRestart != nil {
		fmt.Printf("Last Restart: %s\n", stats.LastRestart.Format(time.RFC3339))
	}

	return nil
}

func init() {
	// Initialize logger
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Initialize container manager
	cm = container.NewDockerManager(logger)

	// Create supervisor with restart policy
	policy := container.RestartPolicy{
		Type:       container.RestartPolicyOnFailure,
		MaxRetries: 3,
	}
	supervisor = container.NewContainerSupervisor(cm, logger, policy)

	// Add CLI commands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(psCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(statsCmd)
}
