package kubernetesobjects

// The containers the platform adds to a Task's Pods, from the cluster Agent's
// --image: the init container that copies the RLark tools in, and the network
// sidecar. Their image is the platform's, not the Task's.
const (
	ToolsInitContainerName      = "rlark-tools-init"
	NetworkSidecarContainerName = "rlark-network-sidecar"
)

// PlatformContainer reports whether name is a container the platform adds.
func PlatformContainer(name string) bool {
	return name == ToolsInitContainerName || name == NetworkSidecarContainerName
}
