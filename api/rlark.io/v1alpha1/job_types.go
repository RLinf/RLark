package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type JobPhase string

const (
	JobPhasePending   JobPhase = "Pending"
	JobPhaseRunning   JobPhase = "Running"
	JobPhaseSucceeded JobPhase = "Succeeded"
	JobPhaseFailed    JobPhase = "Failed"
	JobPhaseStopped   JobPhase = "Stopped"
)

type JobTaskTemplate struct {
	Name     string `json:"name,omitempty"`
	Head     bool   `json:"head,omitempty"`
	TaskSpec `json:",inline"`
}

type JobSpec struct {
	Domain       string            `json:"domain,omitempty"`
	Stopped      bool              `json:"stopped,omitempty"`
	Tasks        []JobTaskTemplate `json:"tasks,omitempty"`
	SSHPublicKey string            `json:"sshPublicKey,omitempty"`
}

type JobTaskStatus struct {
	Name    string    `json:"name,omitempty"`
	Phase   TaskPhase `json:"phase,omitempty"`
	Message string    `json:"message,omitempty"`
}

func (s *JobTaskStatus) GetPhase() string    { return string(s.Phase) }
func (s *JobTaskStatus) SetPhase(p string)   { s.Phase = TaskPhase(p) }
func (s *JobTaskStatus) SetMessage(m string) { s.Message = m }

type JobStatus struct {
	Phase      JobPhase           `json:"phase,omitempty"`
	Tasks      []JobTaskStatus    `json:"tasks,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	StartTime  *metav1.Time       `json:"startTime,omitempty"`
	EndTime    *metav1.Time       `json:"endTime,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=jobs,scope=Cluster,shortName=rljob
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Job struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              JobSpec   `json:"spec,omitempty"`
	Status            JobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type JobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Job `json:"items"`
}
