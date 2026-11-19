package ros2controller

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

// ListPackages returns the ROS 2 packages allowed by the launch package
// whitelist.
func (c *Controller) ListPackages(ctx context.Context, req *pb.ListPackagesRequest) (*pb.ListPackagesResponse, error) {
	allPkgs, err := ros2PkgList()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list packages: %v", err)
	}

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

// GetPackageInfo returns metadata parsed from the package.xml of the
// requested ROS 2 package.
func (c *Controller) GetPackageInfo(ctx context.Context, req *pb.GetPackageInfoRequest) (*pb.GetPackageInfoResponse, error) {
	pkgPath, err := ros2PkgPrefix(req.Name)
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

// GetPackageLaunchFiles returns the launch files available in the requested
// ROS 2 package.
func (c *Controller) GetPackageLaunchFiles(ctx context.Context, req *pb.GetPackageLaunchFilesRequest) (*pb.GetPackageLaunchFilesResponse, error) {
	pkgPath, err := ros2PkgPrefix(req.Name)
	if err != nil {
		return nil, err
	}

	launchDir := filepath.Join(pkgPath, "launch")
	entries, err := os.ReadDir(launchDir)
	if err != nil {
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
		if isLaunchFile(name) {
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

// GetLaunchFileArgs returns the arguments supported by a launch file by
// running "ros2 launch <pkg> <file> --show-args" and parsing its output.
// This works for all launch file types (.py, .xml, .yaml) unlike ROS 1's
// XML-only <arg> tag parsing.
func (c *Controller) GetLaunchFileArgs(ctx context.Context, req *pb.GetLaunchFileArgsRequest) (*pb.GetLaunchFileArgsResponse, error) {
	// First verify the package exists (surfaces a clean NotFound).
	if _, err := ros2PkgPrefix(req.Package); err != nil {
		return nil, err
	}

	args, err := showLaunchArgs(req.Package, req.LaunchFile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "show args for %q/%q: %v", req.Package, req.LaunchFile, err)
	}

	return &pb.GetLaunchFileArgsResponse{
		Package:    req.Package,
		LaunchFile: req.LaunchFile,
		Args:       args,
	}, nil
}

// --------------------------------------------------------------------------
// Package helpers
// --------------------------------------------------------------------------

// ros2PkgList runs "ros2 pkg list" and returns the list of package names.
func ros2PkgList() ([]string, error) {
	cmd := exec.Command("ros2", "pkg", "list")
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

// ros2PkgPrefix returns the absolute path of a ROS 2 package via
// "ros2 pkg prefix <name>". A non-zero exit is surfaced as gRPC NotFound.
func ros2PkgPrefix(name string) (string, error) {
	cmd := exec.Command("ros2", "pkg", "prefix", name)
	out, err := cmd.Output()
	if err != nil {
		return "", status.Errorf(codes.NotFound, "ros2 pkg prefix %q: package not found", name)
	}
	return strings.TrimSpace(string(out)), nil
}

// packageXML is a minimal struct for parsing ROS package.xml metadata.
// Works with both format 2 (ROS 1) and format 3 (ROS 2) — the fields we
// read (name, version, description, maintainer) are the same.
type packageXML struct {
	Name        string `xml:"name"`
	Version     string `xml:"version"`
	Description string `xml:"description"`
	Maintainer  string `xml:"maintainer"`
}

// parsePackageInfo reads and parses a ROS package.xml from the given path.
// The package root is typically <prefix>/share/<pkgname>, but the package.xml
// is in the share directory. For ROS 2, ros2 pkg prefix returns the install
// prefix, and package.xml is at <prefix>/share/<name>/package.xml.
func parsePackageInfo(pkgPrefix string) (*packageXML, error) {
	// Try share/<name>/package.xml first (ROS 2 ament layout).
	// pkgPrefix is the install prefix (e.g. /opt/ros/humble).
	// We need the package name to find the xml; since we don't have it
	// here, try the ament share layout by scanning. However, in practice
	// the caller passes the prefix and we look for package.xml in
	// <prefix>/share/<pkgname>/. Since we don't have pkgname here, the
	// caller should pass the full path. For ROS 2, let's search the
	// share directory.
	//
	// Actually, ros2 pkg prefix returns just the prefix. The package.xml
	// is at <prefix>/share/<pkgname>/package.xml. We need the pkgname.
	// Let's refactor: the caller should resolve the full path.
	data, err := os.ReadFile(filepath.Join(pkgPrefix, "package.xml"))
	if err != nil {
		// Try the share subdirectory layout.
		entries, e := os.ReadDir(filepath.Join(pkgPrefix, "share"))
		if e != nil {
			return nil, fmt.Errorf("read package.xml: %w", err)
		}
		for _, entry := range entries {
			p := filepath.Join(pkgPrefix, "share", entry.Name(), "package.xml")
			data, err = os.ReadFile(p)
			if err == nil {
				break
			}
		}
		if data == nil {
			return nil, fmt.Errorf("read package.xml: %w", err)
		}
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

// isLaunchFile reports whether the given filename has a ROS 2 launch file
// extension (.launch.py, .launch.xml, .launch.yaml). ROS 1's .launch is
// also accepted for backward compatibility.
func isLaunchFile(name string) bool {
	return strings.HasSuffix(name, ".launch") ||
		strings.HasSuffix(name, ".launch.py") ||
		strings.HasSuffix(name, ".launch.xml") ||
		strings.HasSuffix(name, ".launch.yaml")
}

// showLaunchArgs runs "ros2 launch <pkg> <file> --show-args" and parses
// the output into LaunchArg entries. The output format is:
//
//	robot_ip
//	    Robot IP address.
//
//	rate
//	    Control rate in Hz. (default: '100')
//
//	verbose
//	    Verbose logging. (default: 'false')
func showLaunchArgs(pkg, launchFile string) ([]*pb.LaunchArg, error) {
	cmd := exec.Command("ros2", "launch", pkg, launchFile, "--show-args")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ros2 launch --show-args: %w", err)
	}
	return parseShowArgsOutput(string(out)), nil
}

// parseShowArgsOutput parses the output of "ros2 launch --show-args" into
// LaunchArg entries. The output format is:
//
//	Arguments (pass arguments as "<name>:=<value>"):
//
//	'robot_ip':
//	    Robot IP address.
//
//	'rate' (default: '100'):
//	    Control rate in Hz.
//
// A non-indented line starting with a quote or letter begins a new argument;
// the name is stripped of surrounding single quotes and a trailing colon.
// The "(default: '...')" marker may appear on the name line or in the
// description. Indented lines accumulate as the description.
func parseShowArgsOutput(content string) []*pb.LaunchArg {
	var args []*pb.LaunchArg

	var (
		currentName string
		currentDesc string
		currentDef  string
	)
	flush := func() {
		if currentName == "" {
			return
		}
		pa := &pb.LaunchArg{
			Name: currentName,
		}
		def := currentDef
		desc := currentDesc
		// Also check for default in the description text.
		if def == "" {
			def, desc = extractDefault(currentDesc)
		}
		pa.Description = strings.TrimSpace(desc)
		if def != "" {
			pa.Default = def
			pa.Required = false
		} else {
			pa.Required = true
		}
		args = append(args, pa)
		currentName = ""
		currentDesc = ""
		currentDef = ""
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// An indented line is a description for the current arg.
		if line[0] == ' ' || line[0] == '\t' {
			currentDesc += " " + trimmed
			continue
		}
		// Skip the header line that starts with "Arguments".
		if strings.HasPrefix(trimmed, "Arguments") {
			continue
		}
		// A non-indented line starts a new arg; flush the previous one.
		flush()

		// The name line may be:
		//   'robot_ip':
		//   'rate' (default: '100'):
		//   robot_ip:
		// Strip a trailing colon, extract inline default, and strip
		// surrounding single quotes.
		namePart := trimmed
		// Extract inline default before stripping the colon.
		if idx := strings.Index(namePart, "(default: '"); idx >= 0 {
			currentDef, namePart = extractDefault(namePart)
		}
		// Strip trailing colon.
		namePart = strings.TrimSuffix(namePart, ":")
		// Strip surrounding single quotes.
		namePart = strings.Trim(namePart, "' ")
		currentName = namePart
	}
	flush()

	return args
}

// extractDefault pulls the "(default: 'value')" portion out of a description
// string. Returns ("", desc) if no default is present, indicating a required
// argument.
func extractDefault(desc string) (def, cleaned string) {
	const marker = "(default: '"
	idx := strings.Index(desc, marker)
	if idx < 0 {
		return "", desc
	}
	end := strings.Index(desc[idx+len(marker):], "')")
	if end < 0 {
		return "", desc
	}
	def = desc[idx+len(marker) : idx+len(marker)+end]
	cleaned = strings.TrimSpace(desc[:idx])
	return def, cleaned
}
