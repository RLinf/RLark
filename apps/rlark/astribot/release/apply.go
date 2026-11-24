package release

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// Run is one convergence of a site to a desired state.
type Run struct {
	Site    *Site
	Desired State
	// Tree is the release's source; nil when the run goes back, which leaves
	// the definitions as they are (they only ever add optional fields).
	Tree *Tree
	// Back walks the stages in reverse: the Server before the agents.
	Back   bool
	DryRun bool
	// Name is what the run is called in its records and its lease.
	Name string
	// Timeout is how long a stage may take to settle; Poll how often it is read.
	Timeout, Poll time.Duration
	// RollbackCommand is printed with the record of the state before the run.
	RollbackCommand func(before string) string
	Out             io.Writer

	log logTo
	// Images are the images of the release (its images and the environment's installer base), which the images
	// stage copies into the site's registry; none for a run that goes back to a recorded state.
	Images []string

	// installed is the installer this run laid down, which the station-api stage reads back.
	installed installed
	// addonVersions are the Addons ("<cluster>/<addon>") whose version this run has written, and the version: the
	// checks of every later stage, which compare with the baseline taken before the run, take an Addon at that
	// version as the run made it (going back, addons is the first stage, and the agents' stages come after it).
	addonVersions map[string]string
}

type installed struct{ file, sum string }

// StageError is a run stopped at a stage, with what it read there.
type StageError struct {
	Stage   string
	Reading string
	Before  string
}

func (e *StageError) Error() string {
	return fmt.Sprintf("stopped at stage %s: %s", e.Stage, e.Reading)
}

// Stages are the stages of a run in the order it walks them.
func (r *Run) Stages() ([]Stage, error) {
	stages, err := LoadStages(StagesYAML)
	if err != nil {
		return nil, err
	}
	if !r.Back {
		return stages, nil
	}
	back := slices.DeleteFunc(slices.Clone(stages), func(stage Stage) bool { return !stage.Back })
	slices.Reverse(back)
	return back, nil
}

// Plan is every change of every stage, in order.
func (r *Run) Plan(ctx context.Context, current State) ([]Change, error) {
	stages, err := r.Stages()
	if err != nil {
		return nil, err
	}
	// A record given with --to can name a cluster removed since the run that wrote it.
	for name := range r.Desired.Clusters {
		if _, found := current.Clusters[name]; !found {
			return nil, fmt.Errorf("the desired state names cluster %s, which the site does not have", name)
		}
	}
	var all []Change
	for _, stage := range stages {
		changes, err := r.changes(ctx, stage.Name, current)
		if err != nil {
			return nil, err
		}
		all = append(all, changes...)
	}
	return all, nil
}

func (r *Run) changes(ctx context.Context, stage string, current State) ([]Change, error) {
	switch stage {
	case "station-agents":
		changes := Changes(stage, current, r.Desired)
		if r.Tree == nil {
			return changes, nil
		}
		for _, cluster := range slices.Sorted(maps.Keys(r.Desired.Clusters)) {
			retired, err := r.retired(ctx, cluster)
			if err != nil {
				return nil, err
			}
			for _, name := range retired {
				changes = append(changes, Change{Stage: stage, What: cluster + " namespace " + name, From: "present"})
			}
			for _, name := range StationDaemonSets {
				has, made, err := r.stationDaemonSet(ctx, cluster, name)
				if err != nil {
					return nil, err
				}
				if has == nil {
					continue
				}
				// What the cluster's API server makes of it, so that what it fills in is no difference.
				served, err := r.Site.Members[cluster].AppsV1().DaemonSets(stationNamespace).Update(ctx, made, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}})
				if err != nil {
					return nil, fmt.Errorf("cluster %s: DaemonSet %s as the tree states it: %w", cluster, name, err)
				}
				for _, differs := range differences(stationDaemonSetView(has), stationDaemonSetView(served), true) {
					// The images and the pull account are the lines above.
					if !strings.HasSuffix(differs.path, "].image") && !strings.Contains(differs.path, ".imagePullSecrets") {
						changes = append(changes, Change{Stage: stage, What: cluster + " DaemonSet " + name + " " + differs.path, From: differs.from, To: differs.to})
					}
				}
			}
		}
		return changes, nil
	case "images":
		missing, err := r.missingImages(ctx)
		if err != nil {
			return nil, err
		}
		var changes []Change
		for _, image := range missing {
			changes = append(changes, Change{Stage: stage, What: "site registry " + image.repository, To: image.digest})
		}
		return changes, nil
	case "crds-rbac":
		if r.Tree == nil {
			return nil, nil
		}
		changes, err := definitionChanges(ctx, r.Site.Management, r.Tree.Definitions, "", r.Tree.Commit)
		if err != nil {
			return nil, err
		}
		for _, cluster := range slices.Sorted(maps.Keys(r.Site.MemberDefinitions)) {
			member, err := definitionChanges(ctx, r.Site.MemberDefinitions[cluster], r.Tree.MemberDefinitions, cluster, r.Tree.Commit)
			if err != nil {
				return nil, err
			}
			changes = append(changes, member...)
		}
		return changes, nil
	case "gateway", "server", "console":
		desired, err := r.planeDesired(ctx, current)
		if err != nil {
			return nil, err
		}
		return Changes(stage, current, desired), nil
	}
	return Changes(stage, current, r.Desired), nil
}

// planeDesired is the desired state with the control plane's definitions: the hash of each service's definition by
// the stated compose file and values (the machine's own where none are stated) and the service's stated image.
func (r *Run) planeDesired(ctx context.Context, current State) (State, error) {
	desired := r.Desired
	hashes, err := r.Site.Plane.Hashes(ctx, *cmp.Or(desired.Compose, current.Compose), desired.ControlPlane)
	if err != nil {
		return State{}, err
	}
	desired.ControlPlaneConfig = hashes
	return desired, nil
}

// setPlane writes the stated compose file and values where the machine's differ (whichever of the gateway, server
// and console stages a run comes to first: the gateway's going forward, the console's going back) and starts each
// of services anew whose image or definition is not as stated.
func (r *Run) setPlane(ctx context.Context, current State, services ...string) error {
	desired, err := r.planeDesired(ctx, current)
	if err != nil {
		return err
	}
	if want := desired.Compose; want != nil && (want.File != current.Compose.File || !maps.Equal(want.Variables, current.Compose.Variables)) {
		if err := r.Site.Plane.Write(ctx, *want); err != nil {
			return err
		}
		r.log.printf("control plane: compose file and values written")
	}
	for _, service := range services {
		if samePlaneImage(current.ControlPlane[service], desired.ControlPlane[service]) && current.ControlPlaneConfig[service] == desired.ControlPlaneConfig[service] {
			continue
		}
		if err := r.Site.Plane.Set(ctx, service, desired.ControlPlane[service]); err != nil {
			return err
		}
	}
	return nil
}

// Apply walks the stages. It prints the plan; on a dry run it stops there.
// Otherwise it records the state before, holds the site's release lease and
// does each stage: what differs, then the reading back of what it changed
// and the stage's checks against the baseline until they hold or the stage
// times out. It stops at the first stage that does not settle.
func (r *Run) Apply(ctx context.Context) error {
	r.log = logTo{out: r.Out}
	current, err := r.Site.Read(ctx)
	if err != nil {
		return err
	}
	plan, err := r.Plan(ctx, current)
	if err != nil {
		return err
	}
	direction := "forward"
	if r.Back {
		direction = "back"
	}
	r.log.printf("release %s, %s, environment %s: %d changes", r.Name, direction, r.Site.Environment.Name, len(plan))
	for _, change := range plan {
		r.log.printf("  %s", change)
	}
	if err := r.outOfService(ctx); err != nil {
		return err
	}
	if r.DryRun {
		return nil
	}
	if len(plan) == 0 {
		r.log.printf("the site runs release %s as stated; nothing to do", r.Name)
		return r.record(ctx, current)
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	if err := os.MkdirAll(r.Site.Environment.State, 0o700); err != nil {
		return err
	}
	before := filepath.Join(r.Site.Environment.State, stamp+"-before.json")
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(before, data, 0o600); err != nil {
		return err
	}
	record, err := os.Create(filepath.Join(r.Site.Environment.State, stamp+"-"+r.Name+".log"))
	if err != nil {
		return err
	}
	defer record.Close()
	r.log.record = bufio.NewWriter(record)
	r.log.printf("the state before is recorded in %s", before)

	release, err := r.hold(ctx)
	if err != nil {
		return err
	}
	defer release()

	baseline, err := Read(ctx, r.Site.Management, time.Now())
	if err != nil {
		return err
	}
	stages, err := r.Stages()
	if err != nil {
		return err
	}
	for _, stage := range stages {
		if err := r.stage(ctx, stage, baseline); err != nil {
			// Whatever stopped it, the site may be between two states: where, and the way back.
			var stopped *StageError
			if !errors.As(err, &stopped) {
				stopped = &StageError{Stage: stage.Name, Reading: err.Error()}
			}
			stopped.Before = before
			r.log.printf("STOPPED at %s: %s", stopped.Stage, stopped.Reading)
			r.log.printf("to take the site back to how it was: %s", r.RollbackCommand(before))
			r.log.printf("to go on from here: the same command again (a stage that is as the release states is passed)")
			return stopped
		}
	}
	r.log.printf("release %s: every stage done", r.Name)
	return r.record(ctx, current)
}

// record writes the record of the release the site now runs (CurrentRelease), which a run without a tree does not
// state: it leaves the one there is.
func (r *Run) record(ctx context.Context, before State) error {
	switch {
	case r.Desired.Record == nil:
		r.log.printf("the record %s/%s is left as it is: the run has no tree", RecordNamespace, RecordName)
		return nil
	case maps.Equal(before.Record, r.Desired.Record):
		return nil
	}
	if err := writeRecord(ctx, r.Site.Core, r.Desired.Record); err != nil {
		return fmt.Errorf("the record %s/%s: %w", RecordNamespace, RecordName, err)
	}
	r.log.printf("the record %s/%s is release %s", RecordNamespace, RecordName, r.Desired.Record["release"])
	return nil
}

func (r *Run) stage(ctx context.Context, stage Stage, baseline Snapshot) error {
	started := time.Now()
	current, err := r.Site.Read(ctx)
	if err != nil {
		return err
	}
	changes, err := r.changes(ctx, stage.Name, current)
	if err != nil {
		return err
	}
	if stage.Name != "prewarm" && len(changes) == 0 {
		// As stated is not yet done: a run that stopped here left what it wrote, which may not have come up.
		reading, err := r.settled(ctx, stage, baseline)
		if err == nil && reading == "" {
			r.log.printf("%s: as the release states, nothing to do", stage.Name)
			return nil
		}
		r.log.printf("%s: as the release states but not settled (%s): done again", stage.Name, cmp.Or(reading, fmt.Sprint(err)))
	}
	for _, gate := range stage.Gate {
		snapshot, err := Read(ctx, r.Site.Management, time.Now())
		if err != nil {
			return err
		}
		if result := gate.Verify(baseline, snapshot, r.addonVersions); !result.OK {
			return &StageError{Stage: stage.Name, Reading: "gate " + gate.String() + ": " + result.Reading}
		}
	}
	podsBefore, err := r.Site.Pods(ctx)
	if err != nil {
		return err
	}
	r.log.printf("%s: %d changes", stage.Name, len(changes))
	if err := r.act(ctx, stage.Name, current); err != nil {
		return err
	}

	// A reading that fails (a component this stage started anew is not answering yet) is read again.
	if err := r.wait(ctx, stage.Name, r.Timeout*time.Duration(max(stage.TimeoutFactor, 1)), func() (string, error) {
		reading, err := r.settled(ctx, stage, baseline)
		if err != nil {
			return err.Error(), nil
		}
		return reading, nil
	}); err != nil {
		return err
	}

	podsAfter, err := r.Site.Pods(ctx)
	if err != nil {
		return err
	}
	recreated := map[string]int{}
	for uid, owner := range podsAfter {
		if _, was := podsBefore[uid]; !was {
			recreated[owner]++
		}
	}
	// The cluster agents' Deployments are named as each was when it was installed.
	recreates := slices.Clone(stage.Recreates)
	for _, cluster := range current.Clusters {
		if stage.Name == "cluster-agents" {
			recreates = append(recreates, agentNamespace+"/"+cluster.AgentDeployment)
		}
	}
	var made, unexpected []string
	for _, owner := range slices.Sorted(maps.Keys(recreated)) {
		made = append(made, fmt.Sprintf("%s(%d)", owner, recreated[owner]))
		_, namespaced, _ := strings.Cut(owner, " ")
		if !slices.ContainsFunc(recreates, func(pattern string) bool { matched, _ := path.Match(pattern, namespaced); return matched }) {
			unexpected = append(unexpected, owner)
		}
	}
	r.log.printf("%s: done in %s; Pods made anew: %s", stage.Name, time.Now().Sub(started).Round(time.Second), strings.Join(append(made, "none")[:max(len(made), 1)], " "))
	if len(unexpected) > 0 {
		return errors.New("Pods were made anew that this stage does not replace: " + strings.Join(unexpected, ", "))
	}
	return nil
}

// act changes what the stage changes, nothing else.
func (r *Run) act(ctx context.Context, stage string, current State) error {
	site, desired := r.Site, r.Desired
	switch stage {
	case "images":
		missing, err := r.missingImages(ctx)
		if err != nil {
			return err
		}
		for _, image := range missing {
			if _, err := site.Registry.Copy(ctx, site.SiteRegistry, image.repository, image.digest, image.repository, ""); err != nil {
				return fmt.Errorf("%s@%s: %w", image.repository, image.digest, err)
			}
			r.log.printf("images: %s@%s copied into the site's registry", image.repository, short(image.digest))
		}
	case "prewarm":
		// The control plane's images are pulled onto its machine, by the names it pulls them by: one that is not
		// there stops the run before anything changes.
		for _, service := range slices.Sorted(maps.Keys(desired.ControlPlane)) {
			if image := desired.ControlPlane[service]; !samePlaneImage(current.ControlPlane[service], image) {
				if err := site.Plane.Pull(ctx, image); err != nil {
					return fmt.Errorf("%s: %w (the image is to be where the control plane's machine pulls it, environment controlPlane.images)", service, err)
				}
				r.log.printf("prewarm: %s pulled onto the control plane's machine", short(image))
			}
		}
	case "crds-rbac":
		if err := applyDefinitions(ctx, site.Management, r.Tree.Definitions, ""); err != nil {
			return err
		}
		for _, cluster := range slices.Sorted(maps.Keys(site.MemberDefinitions)) {
			if err := applyDefinitions(ctx, site.MemberDefinitions[cluster], r.Tree.MemberDefinitions, cluster); err != nil {
				return err
			}
		}
		return nil
	case "gateway":
		return r.setPlane(ctx, current, "rlark-gateway")
	case "station-api":
		member, pullSecret := site.Members[site.Environment.Station.Cluster], site.Environment.Station.PullSecret
		if desired.Installer == "" || current.Installer == desired.Installer {
			return setDeployment(ctx, member, stationNamespace, stationAPI, desired.StationAPI, pullSecret)
		}
		// The installer first, then both Deployments read it: Station API with its new image, the controller as it is.
		file, sum, err := site.Installer.Put(ctx, desired.Installer)
		if err != nil {
			return fmt.Errorf("lay down the installer %s: %w", short(desired.Installer), err)
		}
		r.installed = installed{file: file, sum: sum}
		r.log.printf("station-api: installer %s is %s, SHA-256 %s", short(desired.Installer), file, sum)
		for name, flag := range map[string]string{stationAPI: apiBundleFlag, stationController: controllerBundleFlag} {
			deployment, err := member.AppsV1().Deployments(stationNamespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if err := pointAtInstaller(deployment, flag, file, desired.Installer); err != nil {
				return err
			}
			if name == stationAPI {
				for i := range deployment.Spec.Template.Spec.Containers {
					deployment.Spec.Template.Spec.Containers[i].Image = desired.StationAPI
				}
			}
			deployment.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: pullSecret}}
			if _, err := member.AppsV1().Deployments(stationNamespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
				return err
			}
		}
	case "controller":
		deployment, err := site.Members[site.Environment.Station.Cluster].AppsV1().Deployments(stationNamespace).Get(ctx, stationController, metav1.GetOptions{})
		if err != nil {
			return err
		}
		container := &deployment.Spec.Template.Spec.Containers[0]
		container.Image = desired.StationController
		deployment.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: site.Environment.Station.PullSecret}}
		_, err = site.Members[site.Environment.Station.Cluster].AppsV1().Deployments(stationNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	case "cluster-agents":
		for _, name := range slices.Sorted(maps.Keys(desired.Clusters)) {
			deployment, err := setAgent(ctx, site.Members[name], desired.Clusters[name])
			if err != nil {
				return fmt.Errorf("cluster %s: %w", name, err)
			}
			// One cluster at a time: the next only once this one's agent is ready.
			if err := r.wait(ctx, stage+": cluster "+name, r.Timeout, func() (string, error) {
				return deploymentSettled(ctx, site.Members[name], agentNamespace, deployment, desired.Clusters[name].Agent)
			}); err != nil {
				return fmt.Errorf("cluster %s: %w", name, err)
			}
		}
	case "node-agents":
		for _, name := range slices.Sorted(maps.Keys(desired.Clusters)) {
			if current.Clusters[name].NodeAgent == desired.Clusters[name].NodeAgent {
				continue
			}
			// The Cluster's declaration of its node agents (spec.nodeAgents, as the Gateway's PUT writes it): what the
			// release does not change (the selector, the rollout) stays as it is.
			if err := r.retryWrite(ctx, func() error {
				clusters := site.Management.Resource(clusterGVR)
				object, err := clusters.Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				declared, _, _ := unstructured.NestedMap(object.Object, "spec", "nodeAgents")
				declared["enabled"], declared["image"] = true, desired.Clusters[name].NodeAgent
				if err := unstructured.SetNestedMap(object.Object, declared, "spec", "nodeAgents"); err != nil {
					return err
				}
				_, err = clusters.Update(ctx, object, metav1.UpdateOptions{})
				return err
			}); err != nil {
				return err
			}
		}
	case "station-agents":
		for _, name := range slices.Sorted(maps.Keys(desired.Clusters)) {
			if r.Tree != nil {
				retired, err := r.retired(ctx, name)
				if err != nil {
					return fmt.Errorf("cluster %s: %w", name, err)
				}
				for _, namespace := range retired {
					if err := site.Members[name].CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
						return fmt.Errorf("cluster %s: namespace %s: %w", name, namespace, err)
					}
				}
			}
			for _, daemonSet := range StationDaemonSets {
				// The DaemonSet's controller writes its status meanwhile: a write that conflicts reads it anew.
				if err := r.retryWrite(ctx, func() error {
					has, made, err := r.stationDaemonSet(ctx, name, daemonSet)
					if err != nil || has == nil {
						return err
					}
					_, err = site.Members[name].AppsV1().DaemonSets(stationNamespace).Update(ctx, made, metav1.UpdateOptions{})
					return err
				}); err != nil {
					return fmt.Errorf("cluster %s: %w", name, err)
				}
			}
		}
	case "server":
		return r.setPlane(ctx, current, "rlark-server", "rlark-controller-manager")
	case "console":
		return r.setPlane(ctx, current, "rlark-ui")
	case "addons":
		for _, cluster := range slices.Sorted(maps.Keys(desired.Clusters)) {
			for _, name := range addonOrder(desired.Clusters[cluster].Addons, r.Desired) {
				was, installed := current.Clusters[cluster].Addons[name]
				want := Merged(was, desired.Clusters[cluster].Addons[name])
				if installed && was.Version == want.Version && maps.Equal(was.Values, want.Values) {
					continue
				}
				if err := r.retryWrite(ctx, func() error { return site.PutAddon(ctx, cluster, name, want, !installed) }); err != nil {
					return err
				}
				if was.Version != want.Version {
					if r.addonVersions == nil {
						r.addonVersions = map[string]string{}
					}
					r.addonVersions[cluster+"/"+name] = want.Version
				}
				// An Addon others pull through (a site's cache) is ready before the next is written.
				if err := r.wait(ctx, stage+": "+cluster+"/"+name, r.Timeout, func() (string, error) { return r.addonSettled(ctx, cluster, name) }); err != nil {
					return fmt.Errorf("%s/%s: %w", cluster, name, err)
				}
			}
		}
	}
	return nil
}

// addonOrder is the order Addons are written in: one a release says comes
// after another is written after it.
func addonOrder(installed map[string]AddonState, desired State) []string {
	names := slices.Sorted(maps.Keys(installed))
	var ordered []string
	placed := map[string]bool{}
	for len(ordered) < len(names) {
		for _, name := range names {
			if placed[name] || slices.ContainsFunc(desired.AddonsAfter[name], func(before string) bool { _, has := installed[before]; return has && !placed[before] }) {
				continue
			}
			placed[name] = true
			ordered = append(ordered, name)
		}
	}
	return ordered
}

// retryWrite writes until the management plane takes it: a write that conflicts with another (the object
// changed meanwhile) or meets the plane busy or unreachable is asked again until the stage's time is up; one it
// refuses (ErrRefused, forbidden, invalid, absent) is not.
func (r *Run) retryWrite(ctx context.Context, write func() error) error {
	return r.wait(ctx, "a write", r.Timeout, func() (string, error) {
		err := write()
		switch {
		case err == nil:
			return "", nil
		case errors.Is(err, ErrRefused), apierrors.IsForbidden(err), apierrors.IsInvalid(err), apierrors.IsNotFound(err),
			apierrors.IsBadRequest(err), apierrors.IsUnauthorized(err), apierrors.IsAlreadyExists(err):
			return "", err
		}
		return err.Error(), nil
	})
}

// wait reads until reading says nothing is left or the time is up, saying what it waits for (what: …) each time that
// changes: a stage that takes long says why.
func (r *Run) wait(ctx context.Context, what string, timeout time.Duration, reading func() (string, error)) error {
	deadline := time.Now().Add(timeout)
	waiting := ""
	for {
		left, err := reading()
		if err != nil {
			return err
		}
		if left == "" {
			return nil
		}
		if left != waiting {
			r.log.printf("%s: waiting (until %s): %s", what, deadline.Format(time.TimeOnly), left)
			waiting = left
		}
		if time.Now().After(deadline) {
			return errors.New(left)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.Poll):
		}
	}
}

// settled reads back what the stage changed and runs its checks; it says
// what is not yet so, nothing when everything is.
func (r *Run) settled(ctx context.Context, stage Stage, baseline Snapshot) (string, error) {
	site, desired := r.Site, r.Desired
	var left []string
	if stage.Name != "prewarm" {
		current, err := site.Read(ctx)
		if err != nil {
			return "", err
		}
		changes, err := r.changes(ctx, stage.Name, current)
		if err != nil {
			return "", err
		}
		for _, change := range changes {
			left = append(left, "still "+change.String())
		}
	}
	switch stage.Name {
	case "station-api", "controller":
		name := map[string]string{"station-api": stationAPI, "controller": stationController}[stage.Name]
		image := map[string]string{"station-api": desired.StationAPI, "controller": desired.StationController}[stage.Name]
		if reading, err := deploymentSettled(ctx, site.Members[site.Environment.Station.Cluster], stationNamespace, name, image); err != nil || reading != "" {
			return reading, err
		}
		if stage.Name == "station-api" && r.installed.file != "" {
			if reading, err := installerSettled(ctx, site.Members[site.Environment.Station.Cluster], site.Installer, desired.Installer, r.installed.file, r.installed.sum); err != nil || reading != "" {
				return reading, err
			}
		}
	case "node-agents", "station-agents":
		for _, cluster := range slices.Sorted(maps.Keys(desired.Clusters)) {
			want := desired.Clusters[cluster]
			daemonSets := map[string]map[string]string{nodeAgentDaemon: {"": want.NodeAgent}}
			namespace := agentNamespace
			if stage.Name == "station-agents" {
				namespace, daemonSets = stationNamespace, map[string]map[string]string{}
				for _, name := range StationDaemonSets {
					daemonSets[name] = map[string]string{"station-agent": want.StationAgent, "wireguard": want.StationWireguard}
				}
			}
			for _, name := range slices.Sorted(maps.Keys(daemonSets)) {
				reading, err := daemonSetSettled(ctx, site.Members[cluster], namespace, name, daemonSets[name])
				if err != nil {
					return "", err
				}
				if reading != "" {
					left = append(left, cluster+" "+reading)
				}
			}
		}
	case "addons":
		for _, cluster := range slices.Sorted(maps.Keys(desired.Clusters)) {
			for _, name := range slices.Sorted(maps.Keys(desired.Clusters[cluster].Addons)) {
				// One that was not Ready before the run (some sit in Installing on a
				// good day) is not waited for.
				if was, found := baseline.Addons[cluster+"/"+name]; found && was.Phase != "Ready" {
					continue
				}
				reading, err := r.addonSettled(ctx, cluster, name)
				if err != nil {
					return "", err
				}
				if reading != "" {
					left = append(left, cluster+"/"+name+": "+reading)
				}
			}
		}
	}
	if len(left) > 0 {
		return strings.Join(left, "; "), nil
	}

	snapshot, err := Read(ctx, site.Management, time.Now())
	if err != nil {
		return "", err
	}
	for _, check := range stage.Check {
		if result := check.Verify(baseline, snapshot, r.addonVersions); !result.OK {
			left = append(left, check.String()+": "+result.Reading)
		}
	}
	return strings.Join(left, "; "), nil
}

func (r *Run) addonSettled(ctx context.Context, cluster, name string) (string, error) {
	managed, err := r.Site.cluster(ctx, cluster)
	if err != nil {
		return "", err
	}
	object, err := r.Site.Management.Resource(addonGVR).Namespace(managed.Spec.AgentNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	var addon managedAddon
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &addon); err != nil {
		return "", err
	}
	switch {
	case addon.Status.ObservedGeneration < addon.Metadata.Generation:
		return fmt.Sprintf("generation %d not yet observed (%d)", addon.Metadata.Generation, addon.Status.ObservedGeneration), nil
	case addon.Status.Phase != "Ready":
		return fmt.Sprintf("%s %s", addon.Status.Phase, addon.Status.Message), nil
	}
	return "", nil
}

// outOfService prints the nodes the run leaves out of its reading back (outOfService).
func (r *Run) outOfService(ctx context.Context) error {
	for _, cluster := range slices.Sorted(maps.Keys(r.Desired.Clusters)) {
		nodes, err := r.Site.Members[cluster].CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, node := range nodes.Items {
			if why := outOfService(node); why != "" {
				r.log.printf("  %s/%s is %s: left out of the read-back", cluster, node.Name, why)
			}
		}
	}
	return nil
}

// outOfService: why a node is out of service, "" when it is in. A cordoned node is one taken out of service; one
// that is not Ready keeps what it has until it is back. Neither holds a DaemonSet's rollout up.
func outOfService(node corev1.Node) string {
	if node.Spec.Unschedulable {
		return "cordoned"
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return ""
		}
	}
	return "not Ready"
}

// setDeployment gives one of Station's Deployments its image, which it pulls with the site registry's account as
// Station's DaemonSets do: the node it runs on need not have had the image pulled for it.
func setDeployment(ctx context.Context, member kubernetes.Interface, namespace, name, image, pullSecret string) error {
	deployment, err := member.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for i := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[i].Image = image
	}
	deployment.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: pullSecret}}
	_, err = member.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

// agentReadyPort is where the cluster agent answers /readyz.
const agentReadyPort = 10382

// setAgent gives the cluster agent its image, the tools it hands to Tasks and its readiness probe. The agent is one
// Pod on its node's network: the one before goes before the next comes, or the two want the same ports. It returns
// the agent's Deployment's name.
func setAgent(ctx context.Context, member kubernetes.Interface, want ClusterState) (string, error) {
	deployment, at, err := clusterAgent(ctx, member)
	if err != nil {
		return "", err
	}
	container := &deployment.Spec.Template.Spec.Containers[at]
	container.Image = want.Agent
	for i, arg := range container.Args {
		switch {
		case strings.HasPrefix(arg, "--image="):
			container.Args[i] = "--image=" + want.Tools
		case arg == "--image" && i+1 < len(container.Args):
			container.Args[i+1] = want.Tools
		}
	}
	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler:  corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/readyz", Port: intstr.FromInt32(agentReadyPort)}},
		PeriodSeconds: 5, TimeoutSeconds: 3, FailureThreshold: 3,
	}
	if deployment.Spec.Template.Spec.HostNetwork {
		deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	}
	_, err = member.AppsV1().Deployments(agentNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return deployment.Name, err
}

// stationDaemonSet is one of Station's DaemonSets as a cluster has it and as the run makes it; neither where the
// cluster does not run it. A run with a tree makes it what the tree's station-agent.yaml states, the whole spec but
// the selector (which a DaemonSet keeps), with the release's images: the template and the binaries it starts are one
// commit's and are written at once. A run without one (going back, or to a record) sets its images only. Either
// hands its rollout to the cluster agent (OnDelete, rlark.io/rollout).
func (r *Run) stationDaemonSet(ctx context.Context, cluster, name string) (has, made *appsv1.DaemonSet, err error) {
	has, err = r.Site.Members[cluster].AppsV1().DaemonSets(stationNamespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	want := r.Desired.Clusters[cluster]
	made = has.DeepCopy()
	if r.Tree != nil {
		stated, err := stationDaemonSet(r.Tree.Files["station-agent.yaml"], name, want)
		if err != nil {
			return nil, nil, err
		}
		made.Spec = stated.Spec
		made.Spec.Selector = has.Spec.Selector
	} else {
		for i, container := range made.Spec.Template.Spec.Containers {
			if image, ours := stationContainers[container.Name]; ours {
				made.Spec.Template.Spec.Containers[i].Image = image(want)
			}
		}
	}
	// The cluster's account of the site's registry is the one it pulls with, and no other; none stated leaves it.
	made.Spec.Template.Spec.ImagePullSecrets = has.Spec.Template.Spec.ImagePullSecrets
	if want.StationPullSecret != "" {
		made.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: want.StationPullSecret}}
	}
	made.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}
	if _, set := made.Annotations[rolloutAnnotation]; !set {
		metav1.SetMetaDataAnnotation(&made.ObjectMeta, rolloutAnnotation, "{}")
	}
	return has, made, nil
}

// retired lists the RetiredNamespaces the cluster still has and is not removing already.
func (r *Run) retired(ctx context.Context, cluster string) ([]string, error) {
	var present []string
	for _, name := range RetiredNamespaces {
		namespace, err := r.Site.Members[cluster].CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) || err == nil && namespace.DeletionTimestamp != nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cluster %s: namespace %s: %w", cluster, name, err)
		}
		present = append(present, name)
	}
	return present, nil
}

// stationDaemonSetView is what a run compares of a Station DaemonSet, as JSON has it.
func stationDaemonSetView(daemonSet *appsv1.DaemonSet) any {
	return jsonValue(map[string]any{"metadata": map[string]any{"annotations": map[string]string{rolloutAnnotation: daemonSet.Annotations[rolloutAnnotation]}}, "spec": daemonSet.Spec})
}

// deploymentSettled: every replica is of the current template, runs image
// and is ready.
func deploymentSettled(ctx context.Context, member kubernetes.Interface, namespace, name, image string) (string, error) {
	deployment, err := member.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	status, replicas := deployment.Status, ptr.Deref(deployment.Spec.Replicas, 1)
	switch {
	case !slices.ContainsFunc(deployment.Spec.Template.Spec.Containers, func(container corev1.Container) bool { return container.Image == image }):
		return fmt.Sprintf("%s/%s does not run %s", namespace, name, short(image)), nil
	case status.ObservedGeneration < deployment.Generation:
		return fmt.Sprintf("%s/%s: generation %d not yet observed", namespace, name, deployment.Generation), nil
	case status.UpdatedReplicas != replicas || status.Replicas != replicas || status.ReadyReplicas != replicas:
		return fmt.Sprintf("%s/%s: %d of %d replicas updated, %d ready, %d in all", namespace, name, status.UpdatedReplicas, replicas, status.ReadyReplicas, status.Replicas), nil
	}
	return "", nil
}

// daemonSetSettled: the DaemonSet has no Pod on a node it does not take, and on every node in service its Pod is of
// its current template, runs the images (container name -> image; "" is every container) and is ready. A node out of service (cordoned, or not Ready) holds nobody up.
func daemonSetSettled(ctx context.Context, member kubernetes.Interface, namespace, name string, images map[string]string) (string, error) {
	daemonSet, err := member.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err) && name == nodeAgentDaemon:
		return "the node agents' DaemonSet is not there", nil
	case apierrors.IsNotFound(err):
		// One of Station's the cluster does not run.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	switch status := daemonSet.Status; {
	case status.ObservedGeneration < daemonSet.Generation:
		return fmt.Sprintf("%s/%s: generation %d not yet observed", namespace, name, daemonSet.Generation), nil
	case status.NumberMisscheduled > 0:
		return fmt.Sprintf("%s/%s: %d Pods on nodes it does not take", namespace, name, status.NumberMisscheduled), nil
	}
	selector, err := metav1.LabelSelectorAsSelector(daemonSet.Spec.Selector)
	if err != nil {
		return "", err
	}
	pods, err := member.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", err
	}
	nodes, err := member.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	inService := map[string]bool{}
	for _, node := range nodes.Items {
		inService[node.Name] = outOfService(node) == ""
	}
	var behind []string
	for _, pod := range pods.Items {
		if !inService[pod.Spec.NodeName] {
			continue
		}
		// The DaemonSet controller's own marks, which the cluster agent's rollout replaces Pods by.
		current := podReady(pod) && pod.Labels[templateGenerationLabel] == daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration]
		for _, container := range pod.Spec.Containers {
			if image, named := images[container.Name]; named || images[""] != "" {
				if !named {
					image = images[""]
				}
				current = current && container.Image == image
			}
		}
		if !current {
			behind = append(behind, pod.Spec.NodeName)
		}
	}
	if len(behind) > 0 {
		slices.Sort(behind)
		return fmt.Sprintf("%s/%s not yet replaced and ready on %s", namespace, name, strings.Join(behind, ", ")), nil
	}
	return "", nil
}

func podReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// siteImage is an image of the release by its repository in the registry and its digest.
type siteImage struct{ repository, digest string }

// missingImages are the images of the release, of the registry, that the site's registry does not hold. An image of
// another registry (a release names the registry's images only) is no business of it.
func (r *Run) missingImages(ctx context.Context) ([]siteImage, error) {
	var missing []siteImage
	for _, image := range r.Images {
		name, digest, byDigest := strings.Cut(image, "@")
		repository, ours := strings.CutPrefix(name, r.Site.Registry.Registry+"/")
		if !byDigest || !ours || slices.Contains(missing, siteImage{repository, digest}) {
			continue
		}
		held, err := r.Site.SiteRegistry.has(ctx, "/v2/"+repository+"/manifests/"+digest)
		if err != nil {
			return nil, err
		}
		if !held {
			missing = append(missing, siteImage{repository, digest})
		}
	}
	return missing, nil
}

var leaseGVR = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}

const (
	leaseNamespace = "default"
	leaseName      = "rlark-release"
	leaseSeconds   = 300
)

// hold takes the site's release lease for the run and renews it while the
// run goes on: two people never change one site at once. A lease nobody
// renewed for its duration is taken over.
func (r *Run) hold(ctx context.Context) (func(), error) {
	leases := r.Site.Management.Resource(leaseGVR).Namespace(leaseNamespace)
	me, _ := os.Hostname()
	if account, err := user.Current(); err == nil {
		me = account.Username + "@" + me
	}
	holder := fmt.Sprintf("%s %s", me, r.Name)
	write := func() error {
		now := time.Now().UTC().Format("2006-01-02T15:04:05.000000Z07:00")
		current, err := leases.Get(ctx, leaseName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			lease := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "coordination.k8s.io/v1", "kind": "Lease",
				"metadata": map[string]any{"name": leaseName, "namespace": leaseNamespace},
				"spec":     map[string]any{"holderIdentity": holder, "leaseDurationSeconds": int64(leaseSeconds), "acquireTime": now, "renewTime": now},
			}}
			_, err = leases.Create(ctx, lease, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		other, _, _ := unstructured.NestedString(current.Object, "spec", "holderIdentity")
		renewed, _, _ := unstructured.NestedString(current.Object, "spec", "renewTime")
		if other != "" && other != holder {
			at, _ := time.Parse(time.RFC3339Nano, renewed)
			if time.Now().Sub(at) < leaseSeconds*time.Second {
				return fmt.Errorf("the site's release is held by %q (renewed %s); one run at a time", other, renewed)
			}
		}
		if other != holder {
			_ = unstructured.SetNestedField(current.Object, now, "spec", "acquireTime")
		}
		_ = unstructured.SetNestedField(current.Object, holder, "spec", "holderIdentity")
		_ = unstructured.SetNestedField(current.Object, now, "spec", "renewTime")
		_ = unstructured.SetNestedField(current.Object, int64(leaseSeconds), "spec", "leaseDurationSeconds")
		_, err = leases.Update(ctx, current, metav1.UpdateOptions{})
		return err
	}
	if err := write(); err != nil {
		return nil, err
	}
	renewing, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-renewing.Done():
				return
			case <-time.After(leaseSeconds * time.Second / 5):
				if err := write(); err != nil {
					r.log.printf("the release lease was not renewed: %v", err)
				}
			}
		}
	}()
	return func() {
		stop()
		<-done
		current, err := leases.Get(context.WithoutCancel(ctx), leaseName, metav1.GetOptions{})
		if err != nil {
			return
		}
		if other, _, _ := unstructured.NestedString(current.Object, "spec", "holderIdentity"); other == holder {
			_ = leases.Delete(context.WithoutCancel(ctx), leaseName, metav1.DeleteOptions{})
		}
	}, nil
}
