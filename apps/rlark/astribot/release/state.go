package release

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// State is what an environment runs, in the terms a release states it. The command reads it from the site, derives
// it from an environment and its release (Desired), and records it before it changes anything, so that the record is
// what a run is taken back to.
type State struct {
	// ControlPlane is compose service -> image.
	ControlPlane map[string]string `json:"controlPlane"`
	// Compose is the control plane's compose file and the site's values of it. Stated by a run with a tree (the
	// tree's file, the environment's values) and by a state recorded before a run; none stated leaves them as
	// they are.
	Compose *ComposeFile `json:"compose,omitempty"`
	// ControlPlaneConfig is compose service -> the hash of its definition (docker compose config --hash): its
	// running container's (read), or what the stated file, values and image make of it.
	ControlPlaneConfig map[string]string `json:"controlPlaneConfig,omitempty"`
	StationAPI         string            `json:"stationAPI"`
	// Installer is the artifact of the installer Station API serves and its controller sends.
	Installer         string                  `json:"installer"`
	StationController string                  `json:"stationController"`
	Clusters          map[string]ClusterState `json:"clusters"`
	// AddonsAfter is Addon -> the Addons it is written after.
	AddonsAfter map[string][]string `json:"addonsAfter,omitempty"`
	// Record is the record of the release the site runs (CurrentRelease), written when a run is done; none is
	// stated by a run without a tree, which leaves it as it is.
	Record map[string]string `json:"record,omitempty"`
}

// ComposeFile is the control plane's compose file and its env file but the services' images.
type ComposeFile struct {
	File      string            `json:"file"`
	Variables map[string]string `json:"variables"`
}

type ClusterState struct {
	Agent string `json:"agent"`
	// AgentDeployment is the name of the cluster agent's Deployment, as the site has it (read, not stated).
	AgentDeployment  string `json:"agentDeployment,omitempty"`
	Tools            string `json:"tools"`
	NodeAgent        string `json:"nodeAgent"`
	StationAgent     string `json:"stationAgent"`
	StationWireguard string `json:"stationWireguard"`
	// StationPullSecret is the account every Station DaemonSet of the cluster pulls with (the environment's
	// station.pullSecret), read as the imagePullSecrets each has while they agree. One not stated is not compared.
	StationPullSecret string                `json:"stationPullSecret,omitempty"`
	Addons            map[string]AddonState `json:"addons"`
}

// Change is one thing a stage changes: what, from what, to what.
type Change struct {
	Stage string
	What  string
	From  string
	To    string
}

func (c Change) String() string {
	return fmt.Sprintf("%-16s %s: %s -> %s", c.Stage, c.What, short(c.From), short(c.To))
}

// short keeps a digest readable in a line: the repository and 12 hex digits.
func short(value string) string {
	if at := len(value) - 64; at > 8 && value[at-7:at] == "sha256:" {
		return value[:at+12]
	}
	if len(value) > 120 {
		return value[:117] + "..."
	}
	if value == "" {
		return "(none)"
	}
	return value
}

// samePlaneImage: two references are one image of the control plane's machine when they name the same digest, by
// whichever registry or repository the machine pulls it from (environment controlPlane.images: localhost:5555);
// without digests, when they are equal. A member cluster's image is the name the release states, nothing else: a
// node pulls it by that name, so another registry's name for the same digest is a change.
func samePlaneImage(a, b string) bool {
	_, da, digestA := strings.Cut(a, "@sha256:")
	_, db, digestB := strings.Cut(b, "@sha256:")
	if digestA && digestB {
		return da == db
	}
	return a == b
}

func changed(stage, what, from, to string, into *[]Change) {
	if from != to {
		*into = append(*into, Change{Stage: stage, What: what, From: from, To: to})
	}
}

// Changes is what the named stage changes to bring current to desired.
func Changes(stage string, current, desired State) []Change {
	var out []Change
	clusters := slices.Sorted(maps.Keys(desired.Clusters))
	switch stage {
	case "gateway":
		if desired.Compose != nil {
			changed(stage, "control plane compose file", fileDigest(current.Compose.File), fileDigest(desired.Compose.File), &out)
			variables := maps.Clone(current.Compose.Variables)
			maps.Copy(variables, desired.Compose.Variables)
			for _, key := range slices.Sorted(maps.Keys(variables)) {
				changed(stage, "control plane "+key, current.Compose.Variables[key], desired.Compose.Variables[key], &out)
			}
		}
		changedService(stage, "rlark-gateway", current, desired, &out)
	case "station-api":
		changed(stage, "station-api", current.StationAPI, desired.StationAPI, &out)
		changed(stage, "installer", current.Installer, desired.Installer, &out)
	case "controller":
		changed(stage, "station-controller", current.StationController, desired.StationController, &out)
	case "cluster-agents":
		for _, name := range clusters {
			now, want := current.Clusters[name], desired.Clusters[name]
			changed(stage, name+" agent", now.Agent, want.Agent, &out)
			changed(stage, name+" agent --image", now.Tools, want.Tools, &out)
		}
	case "node-agents":
		for _, name := range clusters {
			changed(stage, name+" node agents", current.Clusters[name].NodeAgent, desired.Clusters[name].NodeAgent, &out)
		}
	case "station-agents":
		for _, name := range clusters {
			// A container the cluster's DaemonSets do not have is not compared: the cluster runs none of it.
			now, want := current.Clusters[name], desired.Clusters[name]
			if now.StationAgent != "" {
				changed(stage, name+" station-agent", now.StationAgent, want.StationAgent, &out)
			}
			if now.StationWireguard != "" {
				changed(stage, name+" station-wireguard", now.StationWireguard, want.StationWireguard, &out)
			}
			if want.StationPullSecret != "" {
				changed(stage, name+" Station DaemonSets' pull secret", now.StationPullSecret, want.StationPullSecret, &out)
			}
		}
	case "server":
		changedService(stage, "rlark-server", current, desired, &out)
		changedService(stage, "rlark-controller-manager", current, desired, &out)
	case "console":
		changedService(stage, "rlark-ui", current, desired, &out)
	case "addons":
		for _, name := range clusters {
			now := current.Clusters[name].Addons
			for _, addon := range slices.Sorted(maps.Keys(desired.Clusters[name].Addons)) {
				want := desired.Clusters[name].Addons[addon]
				was, installed := now[addon]
				if !installed {
					out = append(out, Change{Stage: stage, What: name + "/" + addon, To: "install " + want.Version})
					continue
				}
				changed(stage, name+"/"+addon+" version", was.Version, want.Version, &out)
				// Only the values the release states are its; the others stay as whoever installed the Addon set them.
				for _, key := range slices.Sorted(maps.Keys(want.Values)) {
					changed(stage, name+"/"+addon+" "+key, was.Values[key], want.Values[key], &out)
				}
			}
		}
	}
	return out
}

// Merged is the Addon as it is to be written: its values as they are, with those stated taking their stated value.
func Merged(current, stated AddonState) AddonState {
	values := maps.Clone(current.Values)
	if values == nil {
		values = map[string]string{}
	}
	maps.Copy(values, stated.Values)
	return AddonState{Version: stated.Version, Values: values}
}

// difference is one place a JSON value is not what another states, and what each has there ("" for nothing).
type difference struct{ path, from, to string }

// differences are where actual is not what desired states: a field by its name, an item of a list of named items
// (containers, env, volumes, mounts, a CRD's versions) by its name, of another list by its place. With exact a field
// only actual has is one too; without, actual may hold more than desired states (what an API server fills in).
func differences(actual, desired any, exact bool) []difference {
	var out []difference
	differ("", actual, desired, exact, &out)
	return out
}

func differ(path string, actual, desired any, exact bool, into *[]difference) {
	switch desired := desired.(type) {
	case map[string]any:
		if actual, both := actual.(map[string]any); both {
			keys := maps.Clone(desired)
			if exact {
				maps.Copy(keys, actual)
			}
			for _, key := range slices.Sorted(maps.Keys(keys)) {
				differ(strings.TrimPrefix(path+"."+key, "."), actual[key], desired[key], exact, into)
			}
			return
		}
	case []any:
		actual, both := actual.([]any)
		actualNames, actualItems := namedItems(actual)
		names, items := namedItems(desired)
		switch {
		case both && actualItems != nil && items != nil:
			for _, name := range actualNames {
				differ(path+"["+name+"]", actualItems[name], items[name], exact, into)
			}
			added := slices.DeleteFunc(slices.Clone(names), func(name string) bool { return slices.Contains(actualNames, name) })
			for _, name := range added {
				differ(path+"["+name+"]", nil, items[name], exact, into)
			}
			// The same items in another order (a container's env refers to the ones before it).
			if kept := slices.DeleteFunc(slices.Clone(actualNames), func(name string) bool { return !slices.Contains(names, name) }); len(added) == 0 && len(kept) == len(names) && !slices.Equal(kept, names) {
				*into = append(*into, difference{path + " order", strings.Join(actualNames, ","), strings.Join(names, ",")})
			}
			return
		case both && len(actual) == len(desired):
			for i := range desired {
				differ(fmt.Sprintf("%s[%d]", path, i), actual[i], desired[i], exact, into)
			}
			return
		}
	}
	if !reflect.DeepEqual(actual, desired) {
		*into = append(*into, difference{path, jsonLine(actual), jsonLine(desired)})
	}
}

// namedItems are a list's items by their names, and the names in the list's order; nothing where an item has none or
// two have one.
func namedItems(list []any) ([]string, map[string]any) {
	var names []string
	items := map[string]any{}
	for _, item := range list {
		fields, _ := item.(map[string]any)
		name, named := fields["name"].(string)
		if _, twice := items[name]; !named || twice {
			return nil, nil
		}
		names, items[name] = append(names, name), item
	}
	return names, items
}

func jsonLine(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	}
	data, _ := json.Marshal(value)
	return string(data)
}

// changedService is a control plane service's image, and its definition where the image is the same.
func changedService(stage, service string, current, desired State, out *[]Change) {
	if from, to := current.ControlPlane[service], desired.ControlPlane[service]; !samePlaneImage(from, to) {
		*out = append(*out, Change{Stage: stage, What: service, From: from, To: to})
	} else if want := desired.ControlPlaneConfig[service]; want != "" {
		changed(stage, service+" definition", current.ControlPlaneConfig[service], want, out)
	}
}

// fileDigest names a file's content in a line.
func fileDigest(content string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))
}

// jsonValue is v as encoding/json reads it back: maps of any, slices of any, float64.
func jsonValue(v any) any {
	data, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(data, &out)
	return out
}
