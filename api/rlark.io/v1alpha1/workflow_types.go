package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type WorkflowPhase string

const (
	WorkflowPhasePending   WorkflowPhase = "Pending"
	WorkflowPhaseRunning   WorkflowPhase = "Running"
	WorkflowPhaseSucceeded WorkflowPhase = "Succeeded"
	WorkflowPhaseFailed    WorkflowPhase = "Failed"
)

type WorkflowJobTemplate struct {
	Name         string   `json:"name,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Spec         JobSpec  `json:"spec,omitempty"`
}

type WorkflowSpec struct {
	JobTemplates []WorkflowJobTemplate `json:"jobTemplates,omitempty"`
}

type WorkflowJobStatus struct {
	Name    string   `json:"name,omitempty"`
	Phase   JobPhase `json:"phase,omitempty"`
	Message string   `json:"message,omitempty"`
}

func (s *WorkflowJobStatus) GetPhase() string    { return string(s.Phase) }
func (s *WorkflowJobStatus) SetPhase(p string)   { s.Phase = JobPhase(p) }
func (s *WorkflowJobStatus) SetMessage(m string) { s.Message = m }

type WorkflowStatus struct {
	Phase      WorkflowPhase       `json:"phase,omitempty"`
	Jobs       []WorkflowJobStatus `json:"jobs,omitempty"`
	Conditions []metav1.Condition  `json:"conditions,omitempty"`
	StartTime  *metav1.Time        `json:"startTime,omitempty"`
	EndTime    *metav1.Time        `json:"endTime,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=workflows,scope=Cluster,shortName=rlwf
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkflowSpec   `json:"spec,omitempty"`
	Status            WorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}
