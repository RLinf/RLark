package roscontroller

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --------------------------------------------------------------------------
// ListPackages
// --------------------------------------------------------------------------

// ListPackages returns ROS packages available on the server, filtered by
// the allowed launch packages whitelist.
func (c *Controller) ListPackages(ctx context.Context, req *pb.ListPackagesRequest) (*pb.ListPackagesResponse, error) {
	allPkgs, err := rospackList()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list packages: %v", err)
	}

	// Filter by whitelist.
	var filtered []string
	for _, pkg := range allPkgs {
		if c.isPackageAllowed(pkg) {
			filtered = append(filtered, pkg)
		}
	}
	return &pb.ListPackagesResponse{Packages: filtered}, nil
}

// --------------------------------------------------------------------------
// GetPackageInfo
// --------------------------------------------------------------------------

// GetPackageInfo returns metadata about a ROS package.
func (c *Controller) GetPackageInfo(ctx context.Context, req *pb.GetPackageInfoRequest) (*pb.GetPackageInfoResponse, error) {
	pkgPath, err := rospackFind(req.Name)
	if err != nil {
		return nil, err
	}

	pkg, err := parsePackageInfo(pkgPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "parse package %q: %v", req.Name, err)
	}

	return &pb.GetPackageInfoResponse{
		Info: &pb.PackageInfo{
			Name:        pkg.Name,
			Version:     pkg.Version,
			Description: pkg.Description,
			Maintainer:  pkg.Maintainer,
			Allowed:     c.isPackageAllowed(req.Name),
		},
	}, nil
}

// --------------------------------------------------------------------------
// GetPackageLaunchFiles
// --------------------------------------------------------------------------

// GetPackageLaunchFiles lists launch files in a ROS package.
func (c *Controller) GetPackageLaunchFiles(ctx context.Context, req *pb.GetPackageLaunchFilesRequest) (*pb.GetPackageLaunchFilesResponse, error) {
	pkgPath, err := rospackFind(req.Name)
	if err != nil {
		return nil, err
	}

	launchDir := filepath.Join(pkgPath, "launch")
	entries, err := os.ReadDir(launchDir)
	if err != nil {
		// No launch directory or no access — return empty list, not an error.
		return &pb.GetPackageLaunchFilesResponse{
			Name:        req.Name,
			LaunchFiles: nil,
		}, nil
	}

	var launchFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".launch") || strings.HasSuffix(name, ".launch.py") {
			launchFiles = append(launchFiles, name)
		}
	}

	return &pb.GetPackageLaunchFilesResponse{
		Name:        req.Name,
		LaunchFiles: launchFiles,
	}, nil
}

// --------------------------------------------------------------------------
// GetLaunchFileArgs
// --------------------------------------------------------------------------

// GetLaunchFileArgs returns the arguments supported by a launch file.
func (c *Controller) GetLaunchFileArgs(ctx context.Context, req *pb.GetLaunchFileArgsRequest) (*pb.GetLaunchFileArgsResponse, error) {
	pkgPath, err := rospackFind(req.Package)
	if err != nil {
		return nil, err
	}

	launchPath := filepath.Join(pkgPath, "launch", req.LaunchFile)
	data, err := os.ReadFile(launchPath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "read launch file %q/%q: %v", req.Package, req.LaunchFile, err)
	}

	args := parseLaunchArgs(string(data))
	return &pb.GetLaunchFileArgsResponse{
		Package:    req.Package,
		LaunchFile: req.LaunchFile,
		Args:       args,
	}, nil
}

// --------------------------------------------------------------------------
// Package helpers
// --------------------------------------------------------------------------

// rospackList runs "rospack list-names" and returns the list of package names.
func rospackList() ([]string, error) {
	cmd := exec.Command("rospack", "list-names")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return nil, nil
	}
	return strings.Split(lines, "\n"), nil
}

// rospackFind returns the absolute path of a ROS package. A non-zero exit
// from rospack (most commonly "package not found") is surfaced as a gRPC
// NotFound so HTTP/gRPC clients get a 404 / NotFound rather than 500.
func rospackFind(name string) (string, error) {
	cmd := exec.Command("rospack", "find", name)
	out, err := cmd.Output()
	if err != nil {
		return "", status.Errorf(codes.NotFound, "rospack find %q: package not found", name)
	}
	return strings.TrimSpace(string(out)), nil
}

// packageXML is a minimal struct for parsing ROS package.xml metadata.
type packageXML struct {
	Name        string `xml:"name"`
	Version     string `xml:"version"`
	Description string `xml:"description"`
	Maintainer  string `xml:"maintainer"`
}

// parsePackageInfo reads and parses a ROS package.xml from the given path.
func parsePackageInfo(pkgPath string) (*packageXML, error) {
	data, err := os.ReadFile(filepath.Join(pkgPath, "package.xml"))
	if err != nil {
		return nil, fmt.Errorf("read package.xml: %w", err)
	}
	var pkg packageXML
	if err := xml.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse package.xml: %w", err)
	}
	return &pkg, nil
}

// isPackageAllowed checks if a package name is in the whitelist.
func (c *Controller) isPackageAllowed(name string) bool {
	if c.allowedLaunchPackages == nil {
		return false
	}
	if c.allowedLaunchPackages["*"] {
		return true
	}
	return c.allowedLaunchPackages[name]
}

// launchArgXML is a minimal struct for parsing <arg> tags from launch files.
type launchArgXML struct {
	XMLName  struct{} `xml:"arg"`
	Name     string   `xml:"name,attr"`
	Default  string   `xml:"default,attr"`
	Value    string   `xml:"value,attr"`
	Doc      string   `xml:"doc,attr"`
	DocValue string   `xml:"documentation,attr"`
}

// parseLaunchArgs extracts arg definitions from a ROS launch file XML string.
func parseLaunchArgs(content string) []*pb.LaunchArg {
	var args []*pb.LaunchArg

	decoder := xml.NewDecoder(strings.NewReader(content))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "arg" {
				var arg launchArgXML
				if err := decoder.DecodeElement(&arg, &se); err != nil {
					continue
				}
				// Skip if it's a value-assigned arg (not user-configurable).
				if arg.Value != "" && arg.Default == "" {
					continue
				}
				pa := &pb.LaunchArg{
					Name: arg.Name,
				}
				if arg.Default != "" {
					pa.Default = arg.Default
					pa.Required = false
				} else {
					pa.Required = true
				}
				desc := arg.Doc
				if desc == "" {
					desc = arg.DocValue
				}
				pa.Description = desc
				args = append(args, pa)
			}
		}
	}

	return args
}
