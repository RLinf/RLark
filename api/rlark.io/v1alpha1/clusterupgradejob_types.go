package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterUpgradeJobPhase summarizes the current lifecycle state of a cluster upgrade job.
// +kubebuilder:validation:Enum=Pending;Upgrading;Succeeded;Failed;RolledBack
type ClusterUpgradeJobPhase string

const (
	ClusterUpgradeJobPending    ClusterUpgradeJobPhase = "Pending"
	ClusterUpgradeJobUpgrading  ClusterUpgradeJobPhase = "Upgrading"
	ClusterUpgradeJobSucceeded  ClusterUpgradeJobPhase = "Succeeded"
	ClusterUpgradeJobFailed     ClusterUpgradeJobPhase = "Failed"
	ClusterUpgradeJobRolledBack ClusterUpgradeJobPhase = "RolledBack"
)

// ClusterUpgradeJobSpec defines the desired state of a cluster upgrade job.
// This CR is created by the control plane for each target cluster.
type ClusterUpgradeJobSpec struct {
	// UpgradeJobName is the name of the parent UpgradeJob.
	// +kubebuilder:validation:MinLength=1
	UpgradeJobName string `json:"upgradeJobName"`

	// Image is the target rlarkadm image for upgrade.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ConfigPatch is the configuration patch to apply.
	// +optional
	ConfigPatch string `json:"configPatch,omitempty"`

	// TimeoutSeconds is the timeout for the upgrade.
	// +optional
	// +kubebuilder:default=600
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// AutoRollback indicates whether to automatically rollback on failure.
	// +optional
	// +kubebuilder:default=true
	AutoRollback bool `json:"autoRollback"`
}

// ClusterUpgradeJobStatus defines the observed state of a cluster upgrade job.
// This is updated by the data plane agent.
type ClusterUpgradeJobStatus struct {
	// Phase is the current phase of the upgrade.
	Phase ClusterUpgradeJobPhase `json:"phase,omitempty"`

	// JobName is the name of the local upgrade Job.
	// +optional
	JobName string `json:"jobName,omitempty"`

	// JobNamespace is the namespace of the local upgrade Job.
	// +optional
	JobNamespace string `json:"jobNamespace,omitempty"`

	// Message provides additional information about the current phase.
	// +optional
	Message string `json:"message,omitempty"`

	// RollbackJobName is the name of the rollback Job if rollback was performed.
	// +optional
	RollbackJobName string `json:"rollbackJobName,omitempty"`

	// StartTime is the time when the upgrade started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time when the upgrade completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterupgradejobs,scope=Namespaced,shortName=clusterupjob
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="UpgradeJob",type=string,JSONPath=`.spec.upgradeJobName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ClusterUpgradeJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterUpgradeJobSpec   `json:"spec,omitempty"`
	Status ClusterUpgradeJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterUpgradeJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterUpgradeJob `json:"items"`
}
