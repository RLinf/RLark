package nodepath

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// Mount is one line of a mountinfo table.
type Mount struct {
	// Device is the file system's "major:minor": a bind of it has the same.
	Device     string
	MountPoint string
	FSType     string
}

// MountTable reads the mounts this process sees.
type MountTable func() ([]Mount, error)

// ReadMountInfo reads /proc/self/mountinfo.
func ReadMountInfo() ([]Mount, error) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	return ParseMountInfo(string(data)), nil
}

// ParseMountInfo parses mountinfo lines: "id parent major:minor root
// mountpoint options [optional...] - fstype source superoptions".
func ParseMountInfo(data string) []Mount {
	var mounts []Mount
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if len(fields) < 5 || separator < 0 || separator+1 >= len(fields) {
			continue
		}
		mounts = append(mounts, Mount{Device: fields[2], MountPoint: unescapeMountPath(fields[4]), FSType: fields[separator+1]})
	}
	return mounts
}

// unescapeMountPath undoes mountinfo's octal escapes of space, tab, newline
// and backslash.
func unescapeMountPath(value string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(value)
}

// covering is the mount the path lies on: the longest mount point that is
// the path or one of its parents. Later lines win a tie (a mount stacked on
// another at the same point hides it).
func covering(mounts []Mount, target string) (Mount, bool) {
	var best Mount
	found := false
	for _, mount := range mounts {
		point := mount.MountPoint
		if point != "/" && target != point && !strings.HasPrefix(target, point+"/") {
			continue
		}
		if !found || len(point) >= len(best.MountPoint) {
			best, found = mount, true
		}
	}
	return best, found
}

// rootUnavailableError says why a node cannot serve a root.
type rootUnavailableError struct{ message string }

func (e *rootUnavailableError) Error() string { return e.message }

// CheckRoot finds the mount a storage's root lies on in the node's file
// systems (seen under hostRoot) and requires it to be one of its own, not the
// node's root file system (a directory that is only there because the file
// system did not mount would take the Pod's writes onto the node's system
// disk), and of fsType when given.
func CheckRoot(mounts []Mount, hostRoot, node, root, fsType string) (Mount, error) {
	target := path.Join(hostRoot, root)
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return Mount{}, &rootUnavailableError{fmt.Sprintf("node %s has no directory %s", node, root)}
	}
	mount, found := covering(mounts, target)
	nodeRoot := path.Clean(hostRoot)
	if !found || mount.MountPoint == nodeRoot || mount.MountPoint == "/" {
		return Mount{}, &rootUnavailableError{fmt.Sprintf("node %s: %s is not on a mounted file system (it is on the node's root file system)%s", node, root, expecting(fsType))}
	}
	point := "/" + strings.TrimPrefix(strings.TrimPrefix(mount.MountPoint, nodeRoot), "/")
	if fsType != "" && mount.FSType != fsType {
		return Mount{}, &rootUnavailableError{fmt.Sprintf("node %s: %s is mounted as %s, expected %s", node, point, mount.FSType, fsType)}
	}
	return mount, nil
}

func expecting(fsType string) string {
	if fsType == "" {
		return ""
	}
	return ", expected " + fsType
}
