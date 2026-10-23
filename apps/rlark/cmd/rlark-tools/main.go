package main

import (
	"errors"
	"os"
	"os/signal"
	"syscall"

	rlarklog "github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarktools/raylauncher"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarktools/sshd"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarktools/supervisor"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:     "rlark-tools",
		Short:   "Utilities for RLark workloads",
		Version: version.String(),
	}
	cmd.AddCommand(newSSHDCommand())
	cmd.AddCommand(newSupervisorCommand())
	cmd.AddCommand(newRayLauncherCommand())
	cmd.AddCommand(newIdleCommand())

	if err := cmd.Execute(); err != nil {
		var exitErr *raylauncher.ExitError
		if errors.As(err, &exitErr) && exitErr.Code > 0 && exitErr.Code <= 255 {
			os.Exit(exitErr.Code)
		}
		var supervisorExitErr *supervisor.ExitError
		if errors.As(err, &supervisorExitErr) && supervisorExitErr.Code > 0 && supervisorExitErr.Code <= 255 {
			os.Exit(supervisorExitErr.Code)
		}
		os.Exit(2)
	}
}

func newIdleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "idle",
		Short: "Wait until the workload is terminated",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			<-ctx.Done()
			return nil
		},
	}
}

func newSupervisorCommand() *cobra.Command {
	var path string
	cmd := &cobra.Command{Use: "supervisor", Short: "Supervise workload processes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		config, err := supervisor.Load(path)
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		ctx = rlarklog.WithLogger(ctx, rlarklog.GetLogger().WithName("supervisor"))
		err = supervisor.Run(ctx, config)
		if ctx.Err() != nil {
			return nil
		}
		return err
	}}
	cmd.Flags().StringVar(&path, "config", "/var/run/rlark/processes/supervisor.json", "Supervisor configuration file")
	return cmd
}

func newRayLauncherCommand() *cobra.Command {
	var role, ray, bash, python string
	var prepared bool
	var waitNetwork bool
	cmd := &cobra.Command{Use: "ray-launcher", Short: "Start a Ray workload process", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		err = raylauncher.Run(ctx, raylauncher.Config{Role: role, RayBinary: ray, BashBinary: bash, PythonBinary: python, Prepared: prepared, WaitNetwork: waitNetwork, SelfBinary: self})
		if ctx.Err() != nil {
			return nil
		}
		return err
	}}
	cmd.Flags().StringVar(&role, "role", "", "Ray role: head or worker")
	cmd.Flags().StringVar(&ray, "ray-binary", "ray", "Ray CLI executable")
	cmd.Flags().StringVar(&bash, "bash-binary", "/bin/bash", "Shell used only for compatibility user scripts")
	cmd.Flags().StringVar(&python, "python-binary", "python", "Python executable used for Ray cluster readiness")
	cmd.Flags().BoolVar(&prepared, "prepared", false, "Internal continuation after the compatibility prepare script")
	_ = cmd.Flags().MarkHidden("prepared")
	cmd.Flags().BoolVar(&waitNetwork, "wait-network", false, "Wait for the RLark virtual network before starting Ray")
	return cmd
}

func newSSHDCommand() *cobra.Command {
	var port string
	var shell string
	var authorizedKeysFiles []string
	cmd := &cobra.Command{
		Use:   "sshd",
		Short: "Start the in-pod SSH server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := newSSHDServer(port, shell, authorizedKeysFiles)
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				os.Exit(0)
			}()
			return srv.ListenAndServe()
		},
	}
	cmd.Flags().StringVar(&port, "port", "22", "SSH listen port")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell binary path (default: /bin/bash)")
	cmd.Flags().StringSliceVar(&authorizedKeysFiles, "authorized-key-file", []string{"/var/run/rlark/ssh/authorized_key"}, "Authorized keys file reopened for every authentication attempt; repeat the flag or provide a comma-separated list")
	return cmd
}

func newSSHDServer(port, shell string, authorizedKeysFiles []string) *sshd.Server {
	return &sshd.Server{Port: port, Shell: shell, AuthorizedKeysFiles: append([]string(nil), authorizedKeysFiles...)}
}
