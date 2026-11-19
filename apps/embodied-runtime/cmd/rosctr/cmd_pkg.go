package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func pkgCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pkg",
		Short: "ROS package information",
		Long:  `Query ROS package metadata and launch files from the server.`,
	}

	cmd.AddCommand(
		pkgInfoCmd(socketPath),
		pkgLaunchFilesCmd(socketPath),
		pkgLaunchArgsCmd(socketPath),
	)

	return cmd
}

func pkgInfoCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <package-name>",
		Short: "Show metadata for a ROS package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.GetPackageInfo(ctx, &pb.GetPackageInfoRequest{
				Name: args[0],
			})
			if err != nil {
				return fmt.Errorf("package info: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				info := resp.Info
				fmt.Printf("Name:        %s\n", info.Name)
				fmt.Printf("Version:     %s\n", info.Version)
				fmt.Printf("Description: %s\n", info.Description)
				fmt.Printf("Maintainer:  %s\n", info.Maintainer)
				fmt.Printf("Allowed:     %v\n", info.Allowed)
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}

func pkgLaunchFilesCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "launch-files <package-name>",
		Short: "List launch files in a ROS package",
		Long:  `List .launch and .launch.py files in the package's launch/ directory.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.GetPackageLaunchFiles(ctx, &pb.GetPackageLaunchFilesRequest{
				Name: args[0],
			})
			if err != nil {
				return fmt.Errorf("launch files: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if len(resp.LaunchFiles) == 0 {
					fmt.Println("(no launch files)")
					return
				}
				for _, f := range resp.LaunchFiles {
					fmt.Println(f)
				}
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}

func pkgLaunchArgsCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "launch-args <package-name> <launch-file>",
		Short: "Show arguments supported by a launch file",
		Long:  `Parse and display the <arg> tags defined in a ROS launch file.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.GetLaunchFileArgs(ctx, &pb.GetLaunchFileArgsRequest{
				Package:    args[0],
				LaunchFile: args[1],
			})
			if err != nil {
				return fmt.Errorf("launch args: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if len(resp.Args) == 0 {
					fmt.Println("(no arguments)")
					return
				}
				for _, a := range resp.Args {
					mark := " "
					if a.Required {
						mark = "*"
					}
					def := a.Default
					if def == "" {
						def = "(required)"
					}
					desc := a.Description
					if desc != "" {
						desc = "  " + desc
					}
					fmt.Printf("  %c %-20s %s%s\n", mark[0], a.Name, def, desc)
				}
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
