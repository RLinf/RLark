package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// JobPhase 是 Job 的聚合生命周期阶段，由控制面 Job 控制器根据其 Task 的
// 观测状态推导得出。取值集合必须与 Job 状态机保持一致。
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Stopped
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

// JobTag 表示一个任务标签，由 key 和多个 value 组成。
// 标签的个数与 key/value 长度上限不在 CRD Schema 层封闭，由 Gateway 在写入
// 路径上校验，详见开发者文档的 API 约定。
type JobTag struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

type JobSpec struct {
	Domain string `json:"domain,omitempty"`
	// Setting spec.suspended to true starts an orderly stop without deleting the Job resource. The Job becomes Stopped and is archived into the persistence backend after ArchiveTTL (default 5 minutes), while the Job CR is deleted from kcp. Starting the Job via POST /jobs/:name/start restores it from the persistence backend with the same name and spec but a new Kubernetes UID.
	Suspended    bool              `json:"suspended,omitempty"`
	Tasks        []JobTaskTemplate `json:"tasks,omitempty"`
	SSHPublicKey string            `json:"sshPublicKey,omitempty"`
	Tags         []JobTag          `json:"tags,omitempty"`
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
	Phase JobPhase `json:"phase,omitempty"`
	// ObservedGeneration 是 Job 控制器最近一次完成调谐时所观测到的
	// metadata.generation。它是控制器契约而非 API Server 保证：消费者在读取
	// phase / conditions 之前，应比较本字段与 metadata.generation 以识别过期状态。
	ObservedGeneration int64           `json:"observedGeneration,omitempty"`
	Tasks              []JobTaskStatus `json:"tasks,omitempty"`
	// +listType=map
	// +listMapKey=type
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
