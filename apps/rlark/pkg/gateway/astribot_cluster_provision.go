package gateway

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

// A cluster is made from one machine in stages (astribot/docs/install-join.md). Each stage does its work,
// reads back what it did and writes both into the ClusterProvision's status before the next starts; a stage that
// fails stops the making there, and a retry goes on from it. Every stage can be done again: it first looks at
// what the machine and the objects already are.
const (
	stagePrecheck     = "precheck"
	stageBundle       = "bundle"
	stageDeclare      = "declare"
	stageControlPlane = "control-plane"
	stageAgent        = "agent"
	stageStation      = "station"
	stageAddons       = "addons"
	stageAccept       = "accept"
)

var provisionStages = []string{stagePrecheck, stageBundle, stageDeclare, stageControlPlane, stageAgent, stageStation, stageAddons, stageAccept}

// provisionNeedsMachine is whether a retry from stage needs the machine's password again: every stage up to
// the Station's logs in to the machine, or reads the cluster's administrator credential from it.
func provisionNeedsMachine(stage string) bool {
	return slices.Index(provisionStages, stage) <= slices.Index(provisionStages, stageStation)
}

// provisionRun is one making of a cluster, from the stage it starts at to the last or to the one that fails. The
// password is held here only, and cleared when the run ends.
type provisionRun struct {
	g        *Gateway
	name     string
	password []byte

	machine provisionMachine
	// admin reaches the new cluster's API with its administrator's credential, read from the machine.
	admin *rest.Config
	// progress is the latest line of a long stage, written into the status now and then.
	progress     string
	progressSent time.Time
}

// stagedRuns are the makings of clusters, or the joins of machines, in progress, by name: one at a time for each.
type stagedRuns struct {
	mu      sync.Mutex
	running map[string]bool
}

func (r *stagedRuns) start(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running == nil {
		r.running = map[string]bool{}
	}
	if r.running[name] {
		return false
	}
	r.running[name] = true
	return true
}

func (r *stagedRuns) end(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, name)
}

// provisionAgentID is the agent's id of a cluster: its name without the namespace prefix.
func provisionAgentID(cluster string) string {
	return strings.TrimPrefix(cluster, apis.RLarkAgentNamespacePrefix)
}

// interruptProvisions marks every making that was running when the Gateway stopped: it stopped where it was, and
// is retried from there.
func (g *Gateway) interruptProvisions(ctx context.Context) error {
	provisions, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list cluster provisions: %w", err)
	}
	for i := range provisions.Items {
		p := &provisions.Items[i]
		if p.Status.Phase != rlarkv1alpha1.ClusterProvisionRunning {
			continue
		}
		stage := p.Status.Stage
		if err := g.writeProvisionStatus(ctx, p.Name, func(status *rlarkv1alpha1.ClusterProvisionStatus) {
			status.Phase = rlarkv1alpha1.ClusterProvisionFailed
			setStage(&status.Stages, &status.Stage, stage, rlarkv1alpha1.ClusterProvisionStageFailed, "", "the Gateway restarted during this stage; retry goes on from it")
		}); err != nil {
			return err
		}
	}
	return nil
}

// writeProvisionStatus changes the status of a ClusterProvision as it is now.
func (g *Gateway) writeProvisionStatus(ctx context.Context, name string, change func(*rlarkv1alpha1.ClusterProvisionStatus)) error {
	client := g.kubeClient.RlinfV1alpha1().ClusterProvisions()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		p, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		change(&p.Status)
		_, err = client.UpdateStatus(ctx, p, metav1.UpdateOptions{})
		return err
	})
}

// setStage records where one stage of stages stands, and makes it the current one.
func setStage(stages *[]rlarkv1alpha1.ClusterProvisionStage, current *string, name string, state rlarkv1alpha1.ClusterProvisionStageState, readback, message string) {
	now := metav1.Now()
	i := slices.IndexFunc(*stages, func(s rlarkv1alpha1.ClusterProvisionStage) bool { return s.Name == name })
	if i < 0 {
		*stages = append(*stages, rlarkv1alpha1.ClusterProvisionStage{Name: name})
		i = len(*stages) - 1
	}
	stage := &(*stages)[i]
	stage.State, stage.Message = state, message
	switch state {
	case rlarkv1alpha1.ClusterProvisionStageRunning:
		stage.StartedAt, stage.FinishedAt, stage.Readback = &now, nil, ""
	case rlarkv1alpha1.ClusterProvisionStageDone, rlarkv1alpha1.ClusterProvisionStageSkipped, rlarkv1alpha1.ClusterProvisionStageFailed:
		stage.FinishedAt, stage.Readback = &now, readback
	}
	*current = name
}

// newProvisionStatus is the status of a making that has not started: every stage pending.
func newProvisionStatus() rlarkv1alpha1.ClusterProvisionStatus {
	status := rlarkv1alpha1.ClusterProvisionStatus{Phase: rlarkv1alpha1.ClusterProvisionRunning, Stage: provisionStages[0]}
	for _, name := range provisionStages {
		status.Stages = append(status.Stages, rlarkv1alpha1.ClusterProvisionStage{Name: name, State: rlarkv1alpha1.ClusterProvisionStagePending})
	}
	return status
}

// provisionStageFuncs are the stages' work. Each returns what it read back; it records what it found or chose in
// p.Status.Facts, which is written with its stage.
var provisionStageFuncs = map[string]func(*provisionRun, context.Context, *rlarkv1alpha1.ClusterProvision) (string, error){
	stagePrecheck:     (*provisionRun).precheck,
	stageBundle:       (*provisionRun).bundle,
	stageDeclare:      (*provisionRun).declare,
	stageControlPlane: (*provisionRun).controlPlane,
	stageAgent:        (*provisionRun).agent,
	stageStation:      (*provisionRun).station,
	stageAddons:       (*provisionRun).addons,
	stageAccept:       (*provisionRun).accept,
}

// run makes the cluster from stage on. It is started by the request, with a context of its own.
func (r *provisionRun) run(ctx context.Context, from string) {
	logger := log.FromContext(ctx).WithValues("clusterProvision", r.name)
	defer r.g.provisions.end(r.name)
	defer clear(r.password)
	defer func() {
		if r.machine != nil {
			_ = r.machine.Close()
		}
	}()
	for _, stage := range provisionStages[slices.Index(provisionStages, from):] {
		if err := r.g.writeProvisionStatus(ctx, r.name, func(status *rlarkv1alpha1.ClusterProvisionStatus) {
			status.Phase = rlarkv1alpha1.ClusterProvisionRunning
			setStage(&status.Stages, &status.Stage, stage, rlarkv1alpha1.ClusterProvisionStageRunning, "", "")
		}); err != nil {
			logger.Error(err, "write the stage's start", "stage", stage)
			return
		}
		p, err := r.g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(ctx, r.name, metav1.GetOptions{})
		if err != nil {
			logger.Error(err, "read the cluster provision", "stage", stage)
			return
		}
		r.progress = ""
		readback, err := provisionStageFuncs[stage](r, ctx, p)
		facts := p.Status.Facts
		if writeErr := r.g.writeProvisionStatus(ctx, r.name, func(status *rlarkv1alpha1.ClusterProvisionStatus) {
			status.Facts = facts
			if err != nil {
				status.Phase = rlarkv1alpha1.ClusterProvisionFailed
				setStage(&status.Stages, &status.Stage, stage, rlarkv1alpha1.ClusterProvisionStageFailed, readback, err.Error())
				return
			}
			setStage(&status.Stages, &status.Stage, stage, rlarkv1alpha1.ClusterProvisionStageDone, readback, "")
			if stage == provisionStages[len(provisionStages)-1] {
				status.Phase = rlarkv1alpha1.ClusterProvisionSucceeded
			}
		}); writeErr != nil {
			logger.Error(writeErr, "write the stage's end", "stage", stage)
			return
		}
		if err != nil {
			logger.Info("cluster provision stopped", "stage", stage, "reason", err.Error())
			return
		}
		logger.Info("cluster provision stage done", "stage", stage, "readback", readback)
	}
}

// line takes one line of a long stage's progress; the latest is written into the stage's message at most every
// few seconds.
func (r *provisionRun) line(ctx context.Context, stage, text string) {
	r.progress = text
	if time.Since(r.progressSent) < 5*time.Second {
		return
	}
	r.progressSent = time.Now()
	_ = r.g.writeProvisionStatus(ctx, r.name, func(status *rlarkv1alpha1.ClusterProvisionStatus) {
		if i := slices.IndexFunc(status.Stages, func(s rlarkv1alpha1.ClusterProvisionStage) bool { return s.Name == stage }); i >= 0 {
			status.Stages[i].Message = text
		}
	})
}

// needMachine logs in to the machine, once in a run. The host must show the key it showed the first time.
func (r *provisionRun) needMachine(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (provisionMachine, error) {
	if r.machine != nil {
		return r.machine, nil
	}
	if len(r.password) == 0 {
		return nil, errors.New("this stage logs in to the machine: retry with its SSH password")
	}
	machine, hostKey, err := r.g.provisionDial(ctx, p.Spec.Machine, r.password, p.Status.HostKey)
	if err != nil {
		return nil, fmt.Errorf("log in to %s@%s: %w", p.Spec.Machine.User, p.Spec.Machine.Address, err)
	}
	if p.Status.HostKey == "" {
		if err := r.g.writeProvisionStatus(ctx, r.name, func(status *rlarkv1alpha1.ClusterProvisionStatus) { status.HostKey = hostKey }); err != nil {
			_ = machine.Close()
			return nil, err
		}
		p.Status.HostKey = hostKey
	}
	r.machine = machine
	return machine, nil
}
