package v1alpha1

// NodeInUseAnnotation is a machine's in-use directory (/run/rlark/in-use) as
// its node Agent reports it on the Kubernetes Node: the names of the files
// there, sorted and joined by commas. Each file is one component that is
// busy on the machine, so an empty value says the machine is idle, and a
// Node without the annotation has not been reported yet and counts as in
// use. Whatever would disturb the machine from the cluster reads it.
const NodeInUseAnnotation = "rlark.io/in-use"
