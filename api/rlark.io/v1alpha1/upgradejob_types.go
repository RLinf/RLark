package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradeJobPhase summarizes the current lifecycle state of an upgrade job.
// +kubebuilder:validation:Enum=Pending;Upgrading;Succeeded;Failed;RollingBack
type UpgradeJobPhase string

const (
	UpgradeJobPending     UpgradeJobPhase = "Pending"
	UpgradeJobUpgrading   UpgradeJobPhase = "Upgrading"
	UpgradeJobSucceeded   UpgradeJobPhase = "Succeeded"
	UpgradeJobFailed      UpgradeJobPhase = "Failed"
	UpgradeJobRollingBack UpgradeJobPhase = "RollingBack"
)

// UpgradeStrategy defines the upgrade strategy.
type UpgradeStrategy struct {
	// Concurrency is the number of clusters to upgrade concurrently.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Concurrency int32 `json:"concurrency,omitempty"`

	// FailureTolerate is the percentage of clusters that can fail before stopping the upgrade.
	// +optional
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	FailureTolerate float64 `json:"failureTolerate,omitempty"`

	// TimeoutSeconds is the timeout for each cluster upgrade.
	// +optional
	// +kubebuilder:default=600
	// +kubebuilder:validation:Minimum=60
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// AutoRollback indicates whether to automatically rollback on failure.
	// +optional
	// +kubebuilder:default=true
	AutoRollback bool `json:"autoRollback"`

	// BackoffLimit is the number of retries before declaring the upgrade failed.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	BackoffLimit int32 `json:"backoffLimit,omitempty"`
}

// MaintenanceWindow defines the maintenance window for the upgrade.
type MaintenanceWindow struct {
	// StartTime is the start time of the maintenance window.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// EndTime is the end time of the maintenance window.
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`
}

// UpgradeJobSpec defines the desired state of an upgrade job.
type UpgradeJobSpec struct {
	// Image is the target rlarkadm image for upgrade.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// ConfigPatch is the full deployment configuration for this upgrade.
	// When empty, the current configuration from rlark-state is used.
	// When set, it must be a complete DeployConfig including apiVersion, kind, and plane.
	// +optional
	ConfigPatch string `json:"configPatch,omitempty"`

	// ClusterSelector selects the target clusters for upgrade.
	// +optional
	ClusterSelector *metav1.LabelSelector `json:"clusterSelector,omitempty"`

	// ClusterNames is the list of target cluster names for upgrade.
	// +optional
	ClusterNames []string `json:"clusterNames,omitempty"`

	// Strategy defines the upgrade strategy.
	// +optional
	Strategy UpgradeStrategy `json:"strategy,omitempty"`

	// MaintenanceWindow defines the maintenance window for the upgrade.
	// +optional
	MaintenanceWindow *MaintenanceWindow `json:"maintenanceWindow,omitempty"`
}

// UpgradeJobCondition reports the per-cluster result of an upgrade job.
type UpgradeJobCondition struct {
	// Type is the type of the condition. Reserved for future use.
	Type string `json:"type"`

	// Cluster is the name of the cluster.
	Cluster string `json:"cluster"`

	// Status is the phase of the ClusterUpgradeJob (Pending, Upgrading, Succeeded, Failed, RolledBack).
	Status string `json:"status"`

	// Reason is a human-readable message about the current status.
	// +optional
	Reason string `json:"reason,omitempty"`

	// TargetImage is the image the cluster was asked to upgrade to.
	TargetImage string `json:"targetImage,omitempty"`

	// RollbackImage is the image the cluster was rolled back to.
	// +optional
	RollbackImage string `json:"rollbackImage,omitempty"`

	// UpgradeTime is when the cluster upgrade completed.
	// +optional
	UpgradeTime *metav1.Time `json:"upgradeTime,omitempty"`
}

// UpgradeJobStatus defines the observed state of an upgrade job.
type UpgradeJobStatus struct {
	// Phase is the current phase of the upgrade job.
	Phase UpgradeJobPhase `json:"phase,omitempty"`

	// TotalClusters is the total number of target clusters.
	TotalClusters int32 `json:"totalClusters,omitempty"`

	// UpgradingClusters is the number of clusters currently being upgraded.
	UpgradingClusters int32 `json:"upgradingClusters,omitempty"`

	// UpgradedClusters is the number of clusters that have been upgraded successfully.
	UpgradedClusters int32 `json:"upgradedClusters,omitempty"`

	// FailedClusters is the number of clusters that failed to upgrade.
	FailedClusters int32 `json:"failedClusters,omitempty"`

	// RollbackClusters is the number of clusters that were rolled back.
	RollbackClusters int32 `json:"rollbackClusters,omitempty"`

	// Conditions is the list of conditions for each cluster.
	// +optional
	// +listType=map
	// +listMapKey=cluster
	Conditions []UpgradeJobCondition `json:"conditions,omitempty"`

	// StartTime is the time when the upgrade job started.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is the time when the upgrade job completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=upgradejobs,scope=Cluster,shortName=upjob
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Upgraded",type=integer,JSONPath=`.status.upgradedClusters`
// +kubebuilder:printcolumn:name="Failed",type=integer,JSONPath=`.status.failedClusters`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalClusters`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type UpgradeJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradeJobSpec   `json:"spec,omitempty"`
	Status UpgradeJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type UpgradeJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpgradeJob `json:"items"`
}
