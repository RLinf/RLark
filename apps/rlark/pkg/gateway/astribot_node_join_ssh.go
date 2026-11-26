package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodessh"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	nodeJoinMachineAnnotation          = "station.rlark.io/machine"
	nodeJoinTypeAnnotation             = "station.rlark.io/machine-type"
	nodeJoinModelAnnotation            = "station.rlark.io/machine-model"
	nodeJoinHostKeyAnnotation          = "station.rlark.io/host-key"
	nodeJoinHostKeyAlgorithmAnnotation = "station.rlark.io/host-key-algorithm"
	nodeJoinInstallLabelsAnnotation    = "station.rlark.io/install-labels"
	nodeJoinNetworkRoleAnnotation      = "station.rlark.io/network-role"
	nodeJoinReasonAnnotation           = "station.rlark.io/join-reason"
	nodeJoinRelayUnavailable           = "RelayUnavailable"
)

func (g *Gateway) machineRelay(ctx context.Context, name string) (string, string, error) {
	machine, err := g.stationClient.Resource(machineGVR).Namespace(machineNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("read scanned Machine %s: %w", name, err)
	}
	site, _, _ := unstructured.NestedString(machine.Object, "spec", "site")
	endpoint, _, _ := unstructured.NestedString(machine.Object, "status", "entry", "via")
	if site == "" || endpoint == "" {
		return "", "", errors.New("the scanned Machine has no site relay endpoint")
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) == nil || port == "" {
		return "", "", errors.New("the scanned Machine has an invalid site relay endpoint")
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", "", errors.New("the scanned Machine has an invalid site relay endpoint")
	}
	secret, err := g.rawClient.CoreV1().Secrets("default").Get(ctx, "station-relay-token-"+site, metav1.GetOptions{})
	if err != nil {
		return endpoint, "", fmt.Errorf("the site relay token for %s is unavailable", site)
	}
	token := string(secret.Data["token"])
	if token == "" {
		return endpoint, "", fmt.Errorf("the site relay token for %s is unavailable", site)
	}
	return endpoint, token, nil
}

func (g *Gateway) clearNodeJoinMachineAdmission(ctx context.Context, j *rlarkv1alpha1.NodeJoin) error {
	name := j.Annotations[nodeJoinMachineAnnotation]
	if name == "" {
		return nil
	}
	return g.clearMachineAdmission(ctx, name, j.Spec.ClusterID, j.Spec.RequestedBy)
}

func (g *Gateway) clearMachineAdmission(ctx context.Context, name, clusterID, actor string) error {
	resource := g.stationClient.Resource(machineGVR).Namespace(machineNamespace)
	object, err := resource.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	admit, found, err := unstructured.NestedMap(object.Object, "spec", "admit")
	if err != nil || !found {
		return err
	}
	if stringValue(admit["clusterID"]) != clusterID || stringValue(admit["type"]) != "Server" || stringValue(admit["requestedBy"]) != actor {
		return fmt.Errorf("Machine %s admission changed; it was left in place", name)
	}
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]string{"resourceVersion": object.GetResourceVersion()},
		"spec":     map[string]interface{}{"admit": nil},
	})
	if err != nil {
		return err
	}
	_, err = resource.Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

func (g *Gateway) dialNodeJoinMachine(ctx context.Context, j *rlarkv1alpha1.NodeJoin, password, sudoPassword []byte) (provisionMachine, string, error) {
	annotations := j.GetAnnotations()
	hostKey := annotations[nodeJoinHostKeyAnnotation]
	if hostKey == "" || annotations[nodeJoinMachineAnnotation] == "" {
		return g.provisionDial(ctx, joinMachine(j), password, j.Status.HostKey)
	}
	machine := joinMachine(j)
	if sudoPassword == nil {
		sudoPassword = password
	}
	connected, shown, err := dialProvisionMachinePinned(ctx, machine, password, sudoPassword, hostKey, annotations[nodeJoinHostKeyAlgorithmAnnotation], "", "")
	if err != nil {
		var failure *nodessh.Error
		if errors.As(err, &failure) && failure.Reason == nodessh.Unreachable {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			endpoint, token, relayErr := g.machineRelay(ctx, annotations[nodeJoinMachineAnnotation])
			if relayErr != nil {
				return nil, "", &nodessh.Error{Reason: nodeJoinRelayUnavailable, Err: fmt.Errorf("直连不可达，站点中继不可用：%v", relayErr)}
			}
			connected, shown, err = dialProvisionMachineRelay(ctx, machine, password, sudoPassword, hostKey, annotations[nodeJoinHostKeyAlgorithmAnnotation], endpoint, token)
			if err != nil {
				return nil, "", err
			}
			return connected, shown, nil
		}
		return nil, "", err
	}
	return connected, shown, nil
}

func machineAdmissionForServer(request createNodeJoinRequest, actor, now string) map[string]interface{} {
	labels := map[string]interface{}{}
	for key, value := range request.Labels {
		labels[key] = value
	}
	admit := map[string]interface{}{
		"clusterID":   request.ClusterID,
		"type":        "Server",
		"requestedBy": actor,
		"requestedAt": now,
	}
	if request.MachineModel != "" {
		admit["model"] = request.MachineModel
	}
	if len(labels) > 0 {
		admit["labels"] = labels
	}
	return admit
}

func nodeJoinAnnotations(request createNodeJoinRequest, hostKey string) (map[string]string, error) {
	labels, err := json.Marshal(request.Labels)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		nodeJoinMachineAnnotation:          request.MachineName,
		nodeJoinTypeAnnotation:             "Server",
		nodeJoinModelAnnotation:            request.MachineModel,
		nodeJoinHostKeyAnnotation:          hostKey,
		nodeJoinHostKeyAlgorithmAnnotation: request.HostKeyAlgorithm,
		nodeJoinInstallLabelsAnnotation:    string(labels),
		nodeJoinNetworkRoleAnnotation:      "Hub",
		nodeJoinReasonAnnotation:           "SSH",
	}, nil
}
