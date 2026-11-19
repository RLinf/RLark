package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type StationPhase string

const (
	StationPhaseBound    StationPhase = "Bound"
	StationPhaseOffline  StationPhase = "Offline"
	StationPhaseConflict StationPhase = "Conflict"
	StationPhaseRetired  StationPhase = "Retired"
)

// Station is the stable inventory record for one Kubernetes node identity.
// Its metadata.name is the business identifier; metadata.uid identifies this
// particular Kubernetes object instance.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=stn
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.binding.nodeName`
// +kubebuilder:printcolumn:name="Node UID",type=string,JSONPath=`.spec.binding.nodeUID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Station struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StationSpec   `json:"spec"`
	Status            StationStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self.displayName == oldSelf.displayName && self.sequence == oldSelf.sequence && (!has(oldSelf.retired) || !oldSelf.retired || (has(self.retired) && self.retired))",message="displayName and sequence are immutable and retirement cannot be reversed"
type StationSpec struct {
	// DisplayName is assigned once and does not follow cluster rename operations.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	DisplayName string `json:"displayName"`
	// Sequence is monotonically allocated within the registered cluster.
	// +kubebuilder:validation:Minimum=1
	Sequence int64          `json:"sequence"`
	Binding  StationBinding `json:"binding"`
	// Retired is durable desired state. Retired stations are never auto-rebound.
	Retired bool `json:"retired,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf || self.version == oldSelf.version + 1",message="a changed binding must increment version exactly once"
type StationBinding struct {
	// NodeName is the last controller-verified Kubernetes Node name.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	NodeName string `json:"nodeName"`
	// NodeUID is the Kubernetes object-instance identity and must be compared exactly.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	NodeUID string `json:"nodeUID"`
	// SystemUUID is supporting evidence only and is never an authorization credential.
	// +kubebuilder:validation:MaxLength=128
	SystemUUID string `json:"systemUUID,omitempty"`
	// Version increases for every administrator-approved rebind.
	// +kubebuilder:validation:Minimum=1
	Version int64 `json:"version"`
	// BoundBy is set by the trusted API/controller, not copied from agent input.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	BoundBy string `json:"boundBy"`
}

type StationStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +kubebuilder:validation:Enum=Bound;Offline;Conflict;Retired
	Phase        StationPhase `json:"phase,omitempty"`
	BootTime     *metav1.Time `json:"bootTime,omitempty"`
	LastSeenTime *metav1.Time `json:"lastSeenTime,omitempty"`
	// Reboots counts the boot-time changes seen since the Station was created.
	// The Rebooted condition describes the last one.
	Reboots int64 `json:"reboots,omitempty"`
	// Conditions are owned by station-controller.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type StationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Station `json:"items"`
}

// StationObservation holds the latest server-validated observation per node.
// Its name is the verified Node name in the namespace mapped to the cluster.
// station-api owns status; agents never write this object directly.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=stnobs
// +kubebuilder:printcolumn:name="Node UID",type=string,JSONPath=`.status.nodeUID`
// +kubebuilder:printcolumn:name="Reported",type=date,JSONPath=`.status.reportedAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type StationObservation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StationObservationSpec   `json:"spec"`
	Status            StationObservationStatus `json:"status,omitempty"`
}

// StationObservationSpec is intentionally empty. The API creates the object
// before writing the validated snapshot through the status subresource.
type StationObservationSpec struct{}

type StationObservationStatus struct {
	// ReportedAt is server receipt time, never a client-provided timestamp.
	ReportedAt metav1.Time `json:"reportedAt"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	NodeUID string `json:"nodeUID"`
	// SystemUUID is evidence only and must not be used to authorize a binding.
	SystemUUID string       `json:"systemUUID,omitempty"`
	BootTime   *metav1.Time `json:"bootTime,omitempty"`
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:items:MaxLength=64
	// +listType=set
	Roles     []string              `json:"roles,omitempty"`
	Wireguard *WireguardObservation `json:"wireguard,omitempty"`
	// +kubebuilder:validation:MaxItems=256
	Probes []StationProbe `json:"probes,omitempty"`
	// Gateway is the station-gateway of the node, as that program states it
	// with every good pass. A node without it runs a gateway that is not
	// reconciling.
	Gateway *GatewayProgram `json:"gateway,omitempty"`
	// Agent is the station-agent that sent this report.
	Agent AgentProgram `json:"agent"`
	// Pressures are the DiskPressure, MemoryPressure and PIDPressure
	// conditions that are True on the member Node, read by station-api when
	// it verified the report. The list is written with every report, so an
	// empty one says the Node is under none and a missing one that the report
	// was stored by a station-api from before the field.
	// +kubebuilder:validation:MaxItems=3
	// +kubebuilder:validation:items:Enum=DiskPressure;MemoryPressure;PIDPressure
	// +listType=set
	Pressures []string `json:"pressures,omitempty"`
}

type GatewayProgram struct {
	// DocumentLevel is the newest configuration document the program
	// understands; it understands every one before it.
	// +kubebuilder:validation:Minimum=1
	DocumentLevel int32 `json:"documentLevel"`
	// Digest is the sha256 of the program's file.
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{64}$`
	Digest string `json:"digest"`
}

type AgentProgram struct {
	// Digest is the sha256 of the program's file.
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{64}$`
	Digest string `json:"digest"`
}

type WireguardObservation struct {
	// Interface is the observed interface name, not a desired configuration.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15
	Interface string `json:"interface"`
	// PublicKey is the public key of the observed interface; private keys are never reported.
	// +kubebuilder:validation:MaxLength=64
	PublicKey string `json:"publicKey,omitempty"`
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=64
	// +listType=set
	Addresses []string `json:"addresses,omitempty"`
	// Peers are the node's WireGuard peers that are enrolled members, as the
	// node's kernel counts them. A Hub's entry for a Leaf is the only account
	// of a Leaf that has dropped off.
	// +kubebuilder:validation:MaxItems=1024
	// +listType=map
	// +listMapKey=name
	Peers []WireguardPeerObservation `json:"peers,omitempty"`
}

type WireguardPeerObservation struct {
	// Name is the peer's member id, "<clusterID>/<nodeName>". station-api
	// assigns it from the public key the node reported: it is that of the
	// Enrolled NetworkEnrollment with this key.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Name            string       `json:"name"`
	LatestHandshake *metav1.Time `json:"latestHandshake,omitempty"`
	// +kubebuilder:validation:Minimum=0
	ReceivedBytes int64 `json:"receivedBytes,omitempty"`
	// +kubebuilder:validation:Minimum=0
	SentBytes int64 `json:"sentBytes,omitempty"`
}

type StationProbe struct {
	// Target is a configured probe label or host:port, bounded by API policy.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Target string `json:"target"`
	// Via identifies the observed route label.
	// +kubebuilder:validation:MaxLength=256
	Via string `json:"via,omitempty"`
	OK  bool   `json:"ok"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=600000
	RTTMillis *int64 `json:"rttMs,omitempty"`
}

// +kubebuilder:object:root=true
type StationObservationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StationObservation `json:"items"`
}

// StationSequence is the per-cluster monotonic Station-number allocator.
// station-controller updates status with resourceVersion-based compare-and-swap.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
type StationSequence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StationSequenceSpec   `json:"spec"`
	Status            StationSequenceStatus `json:"status,omitempty"`
}

type StationSequenceSpec struct {
	// ClusterID is the stable server-registered business ID, not the namespace.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="clusterID is immutable"
	ClusterID string `json:"clusterID"`
}

type StationSequenceStatus struct {
	// LastAssigned may contain gaps; values are never reused.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:XValidation:rule="self >= oldSelf",message="lastAssigned cannot decrease"
	LastAssigned       int64 `json:"lastAssigned"`
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
type StationSequenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StationSequence `json:"items"`
}

// StationRebind is a one-shot, idempotent operation to approve a binding change.
// Audit events are emitted separately and are not inferred from this object's status.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Station",type=string,JSONPath=`.spec.stationRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type StationRebind struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StationRebindSpec   `json:"spec"`
	Status            StationRebindStatus `json:"status,omitempty"`
}

type StationReference struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// UID prevents a same-name replacement from being adopted.
	// +kubebuilder:validation:MinLength=1
	UID string `json:"uid"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="rebind request spec is immutable"
type StationRebindSpec struct {
	// RequestID is stable across retries and unique within the cluster.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	RequestID  string           `json:"requestID"`
	StationRef StationReference `json:"stationRef"`
	// +kubebuilder:validation:Minimum=1
	ExpectedBindingVersion int64 `json:"expectedBindingVersion"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	TargetNodeName string `json:"targetNodeName"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	TargetNodeUID string `json:"targetNodeUID"`
	// +kubebuilder:validation:MaxLength=128
	TargetSystemUUID string `json:"targetSystemUUID,omitempty"`
	// Reason is administrator-provided context, not an audit record.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Reason string `json:"reason"`
	// RequestedBy is resolved from the authenticated administrator at the trusted API boundary.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	RequestedBy string `json:"requestedBy"`
}

type StationRebindStatus struct {
	// +kubebuilder:validation:Enum=Accepted;Applied;Rejected
	Phase              StationRebindPhase `json:"phase,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	// PreviousBinding is written before applying the one-shot change so a retry after restart can audit accurately.
	PreviousBinding *StationBinding `json:"previousBinding,omitempty"`
	CompletedAt     *metav1.Time    `json:"completedAt,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type StationRebindPhase string

const (
	StationRebindPhaseAccepted StationRebindPhase = "Accepted"
	StationRebindPhaseApplied  StationRebindPhase = "Applied"
	StationRebindPhaseRejected StationRebindPhase = "Rejected"
)

// +kubebuilder:object:root=true
type StationRebindList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StationRebind `json:"items"`
}

// StationAuditEvent is an immutable record of a security-sensitive Station operation.
// It is stored separately from operation state and created idempotently by station-controller.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Operation",type=string,JSONPath=`.spec.operation.name`
// +kubebuilder:printcolumn:name="Outcome",type=string,JSONPath=`.spec.outcome`
// +kubebuilder:printcolumn:name="Actor",type=string,JSONPath=`.spec.actor`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.spec.occurredAt`
type StationAuditEvent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              StationAuditEventSpec `json:"spec"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="audit event spec is immutable"
type StationAuditEventSpec struct {
	Operation StationReference `json:"operation"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	OperationID string `json:"operationID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// Delete is of a retired Station: Operation then names the Station itself.
	// +kubebuilder:validation:Enum=Rebind;Delete
	Action string `json:"action"`
	// +kubebuilder:validation:Enum=Applied;Rejected
	Outcome string `json:"outcome"`
	// +kubebuilder:validation:MaxLength=128
	ResultReason string           `json:"resultReason"`
	Station      StationReference `json:"station"`
	// ObservedStationUID records a same-name replacement seen while rejecting an operation.
	// +kubebuilder:validation:MaxLength=128
	ObservedStationUID string `json:"observedStationUID,omitempty"`
	// Actor is resolved from the authenticated API identity.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Actor      string      `json:"actor"`
	OccurredAt metav1.Time `json:"occurredAt"`
	// +kubebuilder:validation:MaxLength=128
	PreviousNodeUID string `json:"previousNodeUID,omitempty"`
	// +kubebuilder:validation:MaxLength=128
	TargetNodeUID          string `json:"targetNodeUID,omitempty"`
	PreviousBindingVersion int64  `json:"previousBindingVersion,omitempty"`
	ResultBindingVersion   int64  `json:"resultBindingVersion,omitempty"`
	// Reason is bounded operator context and must never contain credentials or key material.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Reason string `json:"reason"`
}

// +kubebuilder:object:root=true
type StationAuditEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StationAuditEvent `json:"items"`
}
