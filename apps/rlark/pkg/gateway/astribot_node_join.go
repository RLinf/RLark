package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodessh"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

// A machine is joined to a cluster over SSH in stages, as a cluster is made (astribot_cluster_provision.go): each stage does
// its work, reads back what it did and writes both into the NodeJoin's status before the next starts; a stage that
// fails stops the join there, and a retry goes on from it. The stages that a machine which reaches Station API does
// not need (the installer sent ahead, its declaration as a hub, its own tunnel) are Skipped, with the reason.
const (
	stageTunnel    = "tunnel"
	stageInstall   = "install"
	stageNodeReady = "node-ready"
	stageGPU       = "gpu"
)

var joinStages = []string{stagePrecheck, stageBundle, stageDeclare, stageTunnel, stageInstall, stageNodeReady, stageStation, stageGPU}

// joinNeedsMachine is whether a retry from stage needs the machine's password again: the stages that log in to it,
// and the declaration, which the tunnel follows at once.
func joinNeedsMachine(stage string) bool {
	return slices.Index(joinStages, stage) <= slices.Index(joinStages, stageInstall)
}

// A machine joined by SSH runs the line of its code exactly as an operator would type it, as a unit of its own
// systemd: the join goes on when the Gateway's connection ends, and a retry follows the unit that still runs.
const (
	sshJoinUnit = "rlark-join"
	sshJoinLog  = "/var/log/rlark-join.log"
	// sshJoinExit is the last line the unit writes: the line's exit status.
	sshJoinExit = "rlark-join exit "
	// joinDiskFloor is the free space /var/lib needs for the installer and a node's images.
	joinDiskFloor = 20 << 30
	// joinKeptFor is how long a join that succeeded is listed.
	joinKeptFor = 7 * 24 * time.Hour
)

// tunnelScript puts the tunnel document the SiteNetwork published for the node (stdin) where the installer reads
// it, and starts the site tunnel on it (station-bootstrap tunnel); run again, it changes nothing. Whether the peers
// answer is for awaitTunnel to wait for.
const tunnelScript = `set -eu
umask 077
cat > /etc/rlark/station/bootstrap.json
` + provisionBootstrapDir + `/station-bootstrap tunnel 2>&1
`

// joinStageFuncs are the stages' work. Each returns what it read back, or skipped with why it had nothing to do; it
// records what it found or chose in j.Status.Facts, which is written with its stage.
var joinStageFuncs = map[string]func(*nodeJoinRun, context.Context, *rlarkv1alpha1.NodeJoin) (string, error){
	stagePrecheck:  (*nodeJoinRun).precheck,
	stageBundle:    (*nodeJoinRun).bundle,
	stageDeclare:   (*nodeJoinRun).declare,
	stageTunnel:    (*nodeJoinRun).tunnel,
	stageInstall:   (*nodeJoinRun).install,
	stageNodeReady: (*nodeJoinRun).nodeReady,
	stageStation:   (*nodeJoinRun).station,
	stageGPU:       (*nodeJoinRun).gpu,
}

// skipped is a stage that had nothing to do, and why.
type skipped string

func (s skipped) Error() string { return string(s) }

// nodeJoinRun is one join of a machine, from the stage it starts at to the last or to the one that fails. The
// password is held here only, and cleared when the run ends.
type nodeJoinRun struct {
	g            *Gateway
	name         string
	namespace    string
	password     []byte
	sudoPassword []byte
	machine      provisionMachine

	progressSent time.Time
}

func nodeJoinName(cluster, node string) string { return cluster + "." + node }

// newJoinStatus is the status of a join that has not started: every stage pending.
func newJoinStatus(hostKey string) rlarkv1alpha1.NodeJoinStatus {
	status := rlarkv1alpha1.NodeJoinStatus{Phase: rlarkv1alpha1.ClusterProvisionRunning, Stage: joinStages[0], HostKey: hostKey}
	for _, name := range joinStages {
		status.Stages = append(status.Stages, rlarkv1alpha1.ClusterProvisionStage{Name: name, State: rlarkv1alpha1.ClusterProvisionStagePending})
	}
	return status
}

// writeJoinStatus changes the status of a NodeJoin as it is now.
func (g *Gateway) writeJoinStatus(ctx context.Context, name string, change func(*rlarkv1alpha1.NodeJoinStatus)) error {
	client := g.kubeClient.RlinfV1alpha1().NodeJoins()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		j, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		change(&j.Status)
		_, err = client.UpdateStatus(ctx, j, metav1.UpdateOptions{})
		return err
	})
}

// interruptNodeJoins marks every join that was running when the Gateway stopped: it stopped where it was, and is
// retried from there. A unit rlark-join still running on the machine is followed again by the retry.
func (g *Gateway) interruptNodeJoins(ctx context.Context) error {
	joins, err := g.kubeClient.RlinfV1alpha1().NodeJoins().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list node joins: %w", err)
	}
	for i := range joins.Items {
		j := &joins.Items[i]
		if j.Status.Phase != rlarkv1alpha1.ClusterProvisionRunning {
			continue
		}
		stage := j.Status.Stage
		if err := g.writeJoinStatus(ctx, j.Name, func(status *rlarkv1alpha1.NodeJoinStatus) {
			status.Phase = rlarkv1alpha1.ClusterProvisionFailed
			setStage(&status.Stages, &status.Stage, stage, rlarkv1alpha1.ClusterProvisionStageFailed, "", "Gateway 重启时在这一步，重试从这里继续")
		}); err != nil {
			return err
		}
	}
	return nil
}

// run joins the machine from stage on. It is started by the request, with a context of its own.
func (r *nodeJoinRun) run(ctx context.Context, from string) {
	logger := log.FromContext(ctx).WithValues("nodeJoin", r.name)
	audit := log.FromContext(ctx).WithName("audit")
	defer r.g.nodeJoins.end(r.name)
	defer clear(r.password)
	defer clear(r.sudoPassword)
	defer func() {
		if r.machine != nil {
			_ = r.machine.Close()
		}
	}()
	for _, stage := range joinStages[slices.Index(joinStages, from):] {
		if err := r.g.writeJoinStatus(ctx, r.name, func(status *rlarkv1alpha1.NodeJoinStatus) {
			status.Phase = rlarkv1alpha1.ClusterProvisionRunning
			setStage(&status.Stages, &status.Stage, stage, rlarkv1alpha1.ClusterProvisionStageRunning, "", "")
		}); err != nil {
			logger.Error(err, "write the stage's start", "stage", stage)
			return
		}
		j, err := r.g.kubeClient.RlinfV1alpha1().NodeJoins().Get(ctx, r.name, metav1.GetOptions{})
		if err != nil {
			logger.Error(err, "read the node join", "stage", stage)
			return
		}
		audit.Info("node admission", "action", "ssh-join", "actor", j.Spec.RequestedBy, "cluster", j.Spec.ClusterID, "node", j.Spec.NodeName, "stage", stage, "state", "Running")
		r.progressSent = time.Time{}
		readback, err := joinStageFuncs[stage](r, ctx, j)
		facts := j.Status.Facts
		var reason skipped
		state := rlarkv1alpha1.ClusterProvisionStageDone
		switch {
		case errors.As(err, &reason):
			state, readback, err = rlarkv1alpha1.ClusterProvisionStageSkipped, string(reason), nil
		case err != nil:
			state = rlarkv1alpha1.ClusterProvisionStageFailed
		}
		if writeErr := r.g.writeJoinStatus(ctx, r.name, func(status *rlarkv1alpha1.NodeJoinStatus) {
			status.Facts = facts
			message := ""
			if err != nil {
				status.Phase, message = rlarkv1alpha1.ClusterProvisionFailed, err.Error()
			} else if stage == joinStages[len(joinStages)-1] {
				status.Phase = rlarkv1alpha1.ClusterProvisionSucceeded
			}
			setStage(&status.Stages, &status.Stage, stage, state, readback, message)
		}); writeErr != nil {
			logger.Error(writeErr, "write the stage's end", "stage", stage)
			return
		}
		audit.Info("node admission", "action", "ssh-join", "actor", j.Spec.RequestedBy, "cluster", j.Spec.ClusterID, "node", j.Spec.NodeName, "stage", stage, "state", string(state))
		if err != nil {
			logger.Info("node join stopped", "stage", stage, "reason", err.Error())
			return
		}
	}
}

// line takes one line of a long stage's progress; the latest is written into the stage's message at most every
// five seconds.
func (r *nodeJoinRun) line(ctx context.Context, stage, text string) {
	if time.Since(r.progressSent) < 5*time.Second {
		return
	}
	r.progressSent = time.Now()
	_ = r.g.writeJoinStatus(ctx, r.name, func(status *rlarkv1alpha1.NodeJoinStatus) {
		if i := slices.IndexFunc(status.Stages, func(s rlarkv1alpha1.ClusterProvisionStage) bool { return s.Name == stage }); i >= 0 {
			status.Stages[i].Message = text
		}
	})
}

// joinMachine is the NodeJoin's machine as a login takes it.
func joinMachine(j *rlarkv1alpha1.NodeJoin) rlarkv1alpha1.ClusterProvisionMachine {
	return rlarkv1alpha1.ClusterProvisionMachine{Address: j.Spec.Machine.Address, Port: j.Spec.Machine.Port, User: j.Spec.Machine.User}
}

// needMachine logs in to the machine, once in a run. The host must show the key it showed the first time.
func (r *nodeJoinRun) needMachine(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (provisionMachine, error) {
	if r.machine != nil {
		return r.machine, nil
	}
	if len(r.password) == 0 {
		return nil, errors.New("这一步要登录机器：重试时给出它的 SSH 口令")
	}
	machine, _, err := r.g.dialNodeJoinMachine(ctx, j, r.password, r.sudoPassword)
	if err != nil {
		return nil, fmt.Errorf("登录 %s@%s：%w", j.Spec.Machine.User, j.Spec.Machine.Address, err)
	}
	r.machine = machine
	return machine, nil
}

// joinTunnelEndpoint is where other sites dial the machine's tunnel: the one given, or its address at port 51820.
func joinTunnelEndpoint(j *rlarkv1alpha1.NodeJoin) (string, error) {
	endpoint := j.Spec.Machine.TunnelEndpoint
	if endpoint == "" {
		endpoint = net.JoinHostPort(j.Spec.Machine.Address, strconv.Itoa(provisionTunnelPort))
	}
	if !validTunnelEndpoint(endpoint) {
		return "", fmt.Errorf("机器要经自己的隧道入网，%s 不是 IPv4 ip:port：用主机名登录的机器要给出隧道端点", endpoint)
	}
	return endpoint, nil
}

// addTiming records when one step of the join was seen, once.
func addTiming(facts *rlarkv1alpha1.NodeJoinFacts, event string, at time.Time) {
	if !slices.ContainsFunc(facts.Timings, func(t rlarkv1alpha1.NodeJoinTiming) bool { return t.Event == event }) {
		facts.Timings = append(facts.Timings, rlarkv1alpha1.NodeJoinTiming{Event: event, At: metav1.NewTime(at)})
	}
}

func timingOf(facts rlarkv1alpha1.NodeJoinFacts, event string) (time.Time, bool) {
	for _, t := range facts.Timings {
		if t.Event == event {
			return t.At.Time, true
		}
	}
	return time.Time{}, false
}

func (r *nodeJoinRun) precheck(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	machine, err := r.needMachine(ctx, j)
	if err != nil {
		return "", err
	}
	read, readback, err := readMachine(ctx, machine, joinDiskFloor)
	facts := &j.Status.Facts
	facts.OS, facts.Kernel, facts.GPUs, facts.DiskFree, facts.PrivateAddress = read.OS, read.Kernel, read.GPUs, read.DiskFree, read.PrivateAddress
	facts.BundleSHA256, facts.PublicKey = read.BundleSHA256, read.PublicKey
	login := fmt.Sprintf("%s@%s，主机密钥 %s；节点 %s", j.Spec.Machine.User, net.JoinHostPort(j.Spec.Machine.Address, strconv.Itoa(int(j.Spec.Machine.Port))), j.Status.HostKey, j.Spec.NodeName)
	if err != nil {
		return login + "；" + readback, err
	}
	cluster, node := j.Spec.ClusterID, j.Spec.NodeName
	if taken, err := r.g.nodeNameTaken(ctx, node); err != nil {
		return login, err
	} else if taken != "" && taken != "a node of "+cluster && taken != "declared in the SiteNetwork for "+cluster {
		return login, fmt.Errorf("名为 %s 的节点已经是 %s：换个节点名", node, taken)
	}
	network, err := r.g.siteNetwork(ctx)
	if err != nil {
		return login, fmt.Errorf("read the SiteNetwork: %w", err)
	}
	// The address the code's line will redeem through (joinCommand): the tunnel's, for a machine declared with its
	// key; else the bootstrap address.
	address := r.g.stationAPIFor(network, cluster, node)
	declared := address == r.g.stationBootstrap.TunnelAddress()
	declaredKey := ""
	if declared {
		for _, entry := range (provisionNetwork{network}).list("spec", "nodes") {
			if entry["clusterID"] == cluster && entry["nodeName"] == node {
				declaredKey, _ = entry["publicKey"].(string)
			}
		}
	}
	out, err := machine.Run(ctx, "curl --connect-timeout 10 -sk -o /dev/null -w 'code=%{http_code}\\n' https://"+address+"/v1/bootstrap/ca || echo code=000", nil, nil)
	reached := err == nil && provisionValues(out)["code"] == "200"
	want, err := r.g.stationBootstrap.BundleDigest(ctx)
	if err != nil {
		return login, fmt.Errorf("read the installer's sha256 from Station API: %w", err)
	}
	bundle := "机器上没有安装包"
	switch {
	case facts.BundleSHA256 == want:
		bundle = "安装包 sha " + short(want) + "…（与 Station API 相同）"
	case facts.BundleSHA256 != "":
		bundle = "安装包 sha " + short(facts.BundleSHA256) + "…（Station API 的是 " + short(want) + "…）"
	}
	route := "Station API " + address
	switch {
	case declared && reached && declaredKey == facts.PublicKey:
		facts.Route = rlarkv1alpha1.NodeJoinRouteTunnelUp
		route += " 经本机隧道可达 → 不重建隧道"
	case !declared && reached:
		facts.Route = rlarkv1alpha1.NodeJoinRouteDirect
		route += " 直接可达 → 不需要隧道"
	default:
		facts.Route = rlarkv1alpha1.NodeJoinRouteTunnel
		route += " 不可达 → 先建这台机器自己的隧道"
		if _, err := joinTunnelEndpoint(j); err != nil {
			return login + "；" + route, err
		}
	}
	return strings.Join([]string{login, readback, bundle, route}, "；"), nil
}

// routeSkip is the reason a stage of the tunnel has nothing to do on a machine of route.
func routeSkip(route rlarkv1alpha1.NodeJoinRoute) error {
	if route == rlarkv1alpha1.NodeJoinRouteDirect {
		return skipped("机器直接到得了 Station API，不需要隧道")
	}
	if route == rlarkv1alpha1.NodeJoinRouteTunnelUp {
		return skipped("机器的隧道已经通，声明的密钥和机器上的一致")
	}
	return nil
}

func (r *nodeJoinRun) bundle(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	if err := routeSkip(j.Status.Facts.Route); err != nil {
		return "", err
	}
	release, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	machine, err := r.needMachine(ctx, j)
	if err != nil {
		return "", err
	}
	facts := &j.Status.Facts
	installed, readback, err := r.g.sendInstaller(ctx, machine, release.Registry, facts.BundleSHA256, func(line string) { r.line(ctx, stageBundle, line) })
	facts.BundleSHA256, facts.PublicKey = installed.sha256, installed.publicKey
	return readback, err
}

func (r *nodeJoinRun) declare(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	if err := routeSkip(j.Status.Facts.Route); err != nil {
		return "", err
	}
	facts := &j.Status.Facts
	endpoint, err := joinTunnelEndpoint(j)
	if err != nil {
		return "", err
	}
	cluster, node := j.Spec.ClusterID, j.Spec.NodeName
	address, generation, err := r.g.declareHub(ctx, cluster, node, facts.PrivateAddress, facts.PublicKey, endpoint)
	if err != nil {
		return "", fmt.Errorf("在 SiteNetwork 里声明这台机器：%w", err)
	}
	declaredAt := time.Now()
	facts.TunnelAddress, facts.DeclaredHub, facts.DeclaredGeneration = address, true, generation
	addTiming(facts, "声明 Hub", declaredAt)
	network, err := r.g.provisionNetwork(ctx)
	if err != nil {
		return "", err
	}
	readback := fmt.Sprintf("站点 %s 的 Hub，隧道地址 %s，端点 %s", network.siteOf(cluster), address, endpoint)
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(joinWaitStep) {
		network, err := r.g.provisionNetwork(ctx)
		if err != nil {
			return readback, err
		}
		observed, _, _ := unstructured.NestedInt64(network.object.Object, "status", "observedGeneration")
		if document := network.declared(cluster, node); document != "" && observed >= generation {
			addTiming(facts, "status.declared", time.Now())
			sum := sha256.Sum256([]byte(document))
			return fmt.Sprintf("%s；status.declared 的文档 %s…（声明后 %s 发布）", readback, hex.EncodeToString(sum[:])[:12], time.Since(declaredAt).Round(time.Second)), nil
		}
	}
	return readback, fmt.Errorf("station-controller 2 分钟内没有在 SiteNetwork status.declared 里发布 %s 的隧道文档（第 %d 代）", node, generation)
}

func (r *nodeJoinRun) install(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	machine, err := r.needMachine(ctx, j)
	if err != nil {
		return "", err
	}
	facts := &j.Status.Facts
	cluster, node := j.Spec.ClusterID, j.Spec.NodeName
	admission := types.NamespacedName{Namespace: r.namespace, Name: node}
	out, err := machine.Run(ctx, "systemctl is-active -q "+sshJoinUnit+" && echo active=yes; echo \"last=$(tail -n 1 "+sshJoinLog+" 2>/dev/null)\"", nil, nil)
	if err != nil {
		return "", err
	}
	state := provisionValues(out)
	switch {
	case state["active"] == "yes":
		// A join this NodeJoin started that is still running on the machine (the Gateway restarted): followed again.
	case facts.AdmissionIssuedAt != nil && state["last"] == sshJoinExit+"0":
		return r.installReadback(ctx, admission, sshJoinExit+"0"), nil
	default:
		login := j.Spec.Machine.User + "@" + net.JoinHostPort(j.Spec.Machine.Address, strconv.Itoa(int(j.Spec.Machine.Port)))
		joinLabels := map[string]string{}
		_ = json.Unmarshal([]byte(j.Annotations[nodeJoinInstallLabelsAnnotation]), &joinLabels)
		machineIntent := map[string]interface{}{"category": "Server"}
		if model := j.Annotations[nodeJoinModelAnnotation]; model != "" {
			machineIntent["model"] = model
		}
		issued, _, refused := r.g.admitNode(ctx, j.Spec.RequestedBy, r.namespace, &issueNodeAdmissionRequest{ClusterID: cluster, NodeName: node, Labels: joinLabels, Machine: machineIntent},
			map[string]string{nodeAdmissionSSHLoginAnnotation: login, nodeAdmissionSSHHostKeyAnnotation: j.Status.HostKey})
		if refused != nil {
			return "", fmt.Errorf("发入网码：%w", refused)
		}
		now := metav1.Now()
		facts.AdmissionIssuedAt = &now
		addTiming(facts, "发码", now.Time)
		// Written before the line starts: a retry after a Gateway restart finds the code issued and reads the unit's
		// end instead of issuing another code and joining the machine a second time.
		if err := r.g.writeJoinStatus(ctx, r.name, func(status *rlarkv1alpha1.NodeJoinStatus) {
			status.Facts.AdmissionIssuedAt = &now
			addTiming(&status.Facts, "发码", now.Time)
		}); err != nil {
			return "", fmt.Errorf("记下发码：%w", err)
		}
		// The log of an earlier join goes first: what is followed is this one's.
		if out, err := machine.Run(ctx, "rm -f "+sshJoinLog+" && systemd-run --unit="+sshJoinUnit+" --collect --quiet /bin/sh -c "+
			nodessh.Quote("exec >"+sshJoinLog+" 2>&1; ("+issued.Command+"); echo \""+sshJoinExit+"$?\""), nil, nil); err != nil {
			return "", fmt.Errorf("在机器上启动入网（已经有一个在跑？）：%s", strings.TrimSpace(out))
		}
	}
	last, status := "", ""
	_, err = machine.Run(ctx, "tail -n +1 -F "+sshJoinLog+" 2>/dev/null & t=$!; while systemctl is-active -q "+sshJoinUnit+"; do sleep 1; done; sleep 1; kill $t", nil, func(line string) {
		if code, ended := strings.CutPrefix(line, sshJoinExit); ended {
			status = code
			return
		}
		if line = strings.TrimSpace(line); line != "" {
			last = line
			r.line(ctx, stageInstall, line)
		}
	})
	switch {
	case err != nil:
		return "", fmt.Errorf("没能跟到机器上入网的结束（看机器上的 %s）：%w", sshJoinLog, err)
	case status == "":
		return "", fmt.Errorf("机器上的入网没有写下它的结束（看机器上的 %s）；最后一行：%s", sshJoinLog, last)
	case status != "0":
		failure := "安装程序停了（exit " + status + "）：" + last
		r.g.markInstallFailure(admission, failure)
		return r.installReadback(ctx, admission, sshJoinExit+status), errors.New(failure)
	}
	return r.installReadback(ctx, admission, sshJoinExit+"0；"+last), nil
}

// installReadback is the admission's story of the code, and how the line ended.
func (r *nodeJoinRun) installReadback(ctx context.Context, admission types.NamespacedName, ended string) string {
	object, err := r.g.stationClient.Resource(nodeAdmissionGVR).Namespace(admission.Namespace).Get(ctx, admission.Name, metav1.GetOptions{})
	if err != nil {
		return ended
	}
	view := nodeAdmissionView(object, time.Now())
	readback := fmt.Sprintf("码由 %s 于 %s 发出", view.IssuedBy, view.IssuedAt)
	if view.RedeemedAt != "" {
		readback += fmt.Sprintf("；%s 兑换（admission %s，隧道 %s）", view.RedeemedAt, view.State, view.TunnelAddress)
	}
	return readback + "；" + ended
}

func (g *Gateway) markInstallFailure(admission types.NamespacedName, failure string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if len(failure) > 1000 {
		failure = failure[:1000]
	}
	patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, nodeAdmissionInstallFailureAnnotation, failure))
	if _, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace(admission.Namespace).Patch(ctx, admission.Name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		log.FromContext(ctx).WithName("node-join").Error(err, "mark the install failure on the admission", "admission", admission.String())
	}
}

// joinWaitStep is how often the stages that wait for the management plane look again.
var joinWaitStep = 5 * time.Second

func (r *nodeJoinRun) nodeReady(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	phase := ""
	for deadline := time.Now().Add(5 * time.Minute); ; time.Sleep(joinWaitStep) {
		node, err := r.g.kubeClient.RlinfV1alpha1().Nodes(j.Spec.ClusterID).Get(ctx, j.Spec.NodeName, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			phase = string(node.Status.Phase)
			ready := slices.ContainsFunc(node.Status.Conditions, func(c metav1.Condition) bool {
				return c.Type == rlarkv1alpha1.ConditionTypeReady && c.Status == metav1.ConditionTrue
			})
			if ready && node.Status.Phase == rlarkv1alpha1.NodeOnline {
				capacity := node.Status.Capacity
				return fmt.Sprintf("Ready，%s；%s，内核 %s；%s CPU / %s 内存", phase, node.Status.NodeInfo.OperatingSystem, node.Status.NodeInfo.KernelVersion,
					capacity.Cpu().String(), capacity.Memory().String()), nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("节点 %s 5 分钟后还不是 Ready/Online（phase %q）：看它的 kubelet 和 node-agent", j.Spec.NodeName, phase)
		}
	}
}

func (r *nodeJoinRun) station(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	node := j.Spec.NodeName
	phase, address, state := "", "", ""
	for deadline := time.Now().Add(3 * time.Minute); ; time.Sleep(joinWaitStep) {
		enrollment, err := r.g.stationClient.Resource(networkEnrollmentGVR).Namespace(r.namespace).Get(ctx, node, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			phase, _, _ = unstructured.NestedString(enrollment.Object, "status", "phase")
			address, _, _ = unstructured.NestedString(enrollment.Object, "status", "address")
		}
		admission, err := r.g.stationClient.Resource(nodeAdmissionGVR).Namespace(r.namespace).Get(ctx, node, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return "", err
		}
		if err == nil {
			state = nodeAdmissionView(admission, time.Now()).State
		}
		if phase == "Enrolled" && state == "Joined" {
			return fmt.Sprintf("Enrolled %s；admission Joined", address), nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("3 分钟后入网登记是 %q、admission 是 %q（看节点上 station-agent 的日志）", phase, state)
		}
	}
}

func (r *nodeJoinRun) gpu(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	facts := j.Status.Facts
	if len(facts.GPUs) == 0 {
		return "", skipped("预检没看到 NVIDIA GPU")
	}
	cluster, node := j.Spec.ClusterID, j.Spec.NodeName
	if _, err := r.g.kubeClient.RlinfV1alpha1().Addons(cluster).Get(ctx, "nvidia-gpu", metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return "", fmt.Errorf("集群 %s 没有 nvidia-gpu 插件：先给集群装上它，再重试", cluster)
	} else if err != nil {
		return "", err
	}
	patch := []byte(`{"metadata":{"labels":{"` + acceleratorLabel + `":"nvidia"}}}`)
	if _, err := r.g.kubeClient.RlinfV1alpha1().Nodes(cluster).Patch(ctx, node, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return "", fmt.Errorf("给节点 %s 打标签 %s=nvidia：%w", node, acceleratorLabel, err)
	}
	want := int64(0)
	for _, gpu := range facts.GPUs {
		want += int64(gpu.Count)
	}
	got, err := r.g.nodeGPUs(ctx, cluster, node, want)
	if err != nil {
		return fmt.Sprintf("%s=nvidia；%d nvidia.com/gpu", acceleratorLabel, got), err
	}
	return fmt.Sprintf("%s=nvidia；%d nvidia.com/gpu 可分配（%s）", acceleratorLabel, got, describeGPUs(facts.GPUs)), nil
}

// loginRefused is a refused login or sudo as an answer: 422 for a password the machine refuses (not 401, which would
// end the console's own login).
func loginRefused(failure *nodessh.Error) *requestError {
	status := map[string]int{nodessh.Unreachable: 502, nodeJoinRelayUnavailable: 502, nodessh.RelayRejected: 502, nodessh.LoginRejected: 422}[failure.Reason]
	if status == 0 {
		status = 403
	}
	return &requestError{status: status, reason: failure.Reason, err: failure.Err}
}

var notInNodeName = regexp.MustCompile(`[^a-z0-9.-]+`)

// nodeNameOfHost is the node name a machine's host name gives: lower case, what a node name cannot hold made a dash.
func nodeNameOfHost(hostname string) string {
	return strings.Trim(notInNodeName.ReplaceAllString(strings.ToLower(strings.TrimSpace(hostname)), "-"), "-.")
}
