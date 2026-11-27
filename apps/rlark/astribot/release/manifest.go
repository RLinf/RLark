package release

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// Components are the images a release names, one per component even where
// several are built as one image: each is read back by itself.
var Components = []string{
	"rlark-server", "rlark-controller-manager", "rlark-gateway", "rlark-ui",
	"station-api", "station-controller", "station-agent", "station-wireguard",
	"cluster-agent", "node-agent", "tools",
}

// Release is what a release is, the same for every environment that runs it: the commits it is built from, the image
// of every component and the Addon versions it brings with the values that come with them (its images).
// astribot/release/releases/<name>.yaml.
type Release struct {
	Name   string `json:"name"`
	Source struct {
		RLark string `json:"rlark"`
		Fleet string `json:"fleet,omitempty"`
	} `json:"source"`
	// Images is component -> image by digest. Besides Components it holds the images the Addons refer to
	// ({{images.<name>}}).
	Images map[string]string     `json:"images"`
	Addons map[string]AddonState `json:"addons,omitempty"`

	dir string
}

// AddonState is an Addon's version and values.
type AddonState struct {
	Version string            `json:"version,omitempty"`
	Values  map[string]string `json:"values,omitempty"`
	// After names the Addons this one is written after (a release's only): a site's cache is ready before what pulls
	// through it is.
	After []string `json:"after,omitempty"`
}

// Environment is one site that runs a release: which release and how the command reaches each part of it.
// astribot/release/environments/<name>.yaml.
type Environment struct {
	Name string `json:"name"`
	// Release is the release file, relative to the environment file.
	Release string `json:"release"`
	// Management is the kubeconfig of the command's own identity in kcp (rlarkadm identity issue writes
	// <directory>/rlark-release/kubeconfig, which reads its token from a file beside it).
	Management struct {
		Kubeconfig string `json:"kubeconfig"`
	} `json:"management"`
	// AddonCatalog is the directory of Addon catalog items that are not compiled in, as the Gateway is given it
	// (RLARK_ADDON_CATALOG_DIR): an Addon's Package is published from it as the Gateway would.
	AddonCatalog string `json:"addonCatalog"`
	ControlPlane struct {
		Project string `json:"project"`
		Compose string `json:"compose"`
		Env     string `json:"env"`
		// Images is a release's repository -> the one the control plane's machine pulls it by, the same digest.
		Images map[string]string `json:"images"`
		// Variables are the site's values of the compose file; the env file is these, STATION_API_BOOTSTRAP_URL,
		// ADDONS (Compose) and the services' images.
		Variables map[string]string `json:"variables"`
	} `json:"controlPlane"`
	// Station is the cluster Station API and its controller run in, the account of the site's registry Station's
	// Pods pull with in every cluster (the Secret RLark delivers into station-system under the image registry
	// credential's id), the directory on Station API's machine both Station Deployments read the installer from (and
	// Station API's CA, ca.crt), and Station API as a machine that joins reaches it.
	Station struct {
		Cluster      string `json:"cluster"`
		PullSecret   string `json:"pullSecret"`
		BootstrapDir string `json:"bootstrapDir"`
		BootstrapURL string `json:"bootstrapURL"`
	} `json:"station"`
	// Installer is what the site's installer is built from (astribot/release/run.sh, Dockerfile.installer): an
	// installer of the same Kubernetes, containerd, CNI and image archives by digest, and the file (beside the
	// environment file) of the site registry's CA. A release carries it as its image installer-<environment>.
	Installer struct {
		Base       string `json:"base"`
		RegistryCA string `json:"registryCA"`
	} `json:"installer"`
	Registry Registry `json:"registry"`
	// State is the directory the records of runs go to.
	State string `json:"state"`
}

// Registry is the registry releases are kept in and the site's own, as the command's machine reaches each.
type Registry struct {
	// Name is the registry's name in an image's reference (registry.rlark.internal:5443).
	Name string `json:"name"`
	// Source is the registry releases are kept in (the cloud's), which a release's images are copied into by digest
	// once built (rlarkctl release copy); Site the one the site's own clusters pull from, which a run copies the
	// release's images into before anything pulls them (stage images).
	Source RegistryAccess `json:"source"`
	Site   RegistryAccess `json:"site"`
}

// RegistryAccess is how this machine reaches a registry: its address, with its scheme; the CA an https:// one is
// trusted by (a file beside the environment file, or an absolute path); and its account, an image registry
// credential of the management plane (ir-…), read there by the release's identity.
type RegistryAccess struct {
	Address    string `json:"address"`
	CA         string `json:"ca,omitempty"`
	Credential string `json:"credential,omitempty"`
}

func readYAML(path string, into any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.UnmarshalStrict(data, into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// LoadEnvironment reads an environment file; its release is read with it.
func LoadEnvironment(path string) (Environment, Release, error) {
	var environment Environment
	if err := readYAML(path, &environment); err != nil {
		return Environment{}, Release{}, err
	}
	if ca := &environment.Registry.Source.CA; *ca != "" && !filepath.IsAbs(*ca) {
		*ca = filepath.Join(filepath.Dir(path), *ca)
	}
	release, err := LoadRelease(filepath.Join(filepath.Dir(path), environment.Release))
	return environment, release, err
}

// LoadRelease reads a release file and checks that it names every component by its digest and every image its
// Addons refer to.
func LoadRelease(path string) (Release, error) {
	var release Release
	if err := readYAML(path, &release); err != nil {
		return Release{}, err
	}
	release.dir = filepath.Dir(path)
	for _, component := range Components {
		if !strings.Contains(release.Images[component], "@sha256:") {
			return Release{}, fmt.Errorf("%s: image %s is not named by its digest", path, component)
		}
	}
	for name, addon := range release.Addons {
		if _, err := release.render(addon.Values); err != nil {
			return Release{}, fmt.Errorf("%s: Addon %s: %w", path, name, err)
		}
	}
	return release, nil
}

var reference = regexp.MustCompile(`\{\{images\.([a-z0-9-]+)\}\}`)

// render puts the release's images in place of {{images.<name>}} in values.
func (r Release) render(values map[string]string) (map[string]string, error) {
	rendered := make(map[string]string, len(values))
	var missing []string
	for key, value := range values {
		rendered[key] = reference.ReplaceAllStringFunc(value, func(match string) string {
			name := reference.FindStringSubmatch(match)[1]
			if _, known := r.Images[name]; !known {
				missing = append(missing, name)
			}
			return r.Images[name]
		})
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("names images the release does not have: %s", strings.Join(missing, ", "))
	}
	return rendered, nil
}

// Compose is the control plane's compose file of tree and the site's values of it: the environment's variables,
// STATION_API_BOOTSTRAP_URL its station.bootstrapURL and ADDONS its addonCatalog.
func (e Environment) Compose(tree Tree) *ComposeFile {
	variables := maps.Clone(e.ControlPlane.Variables)
	if variables == nil {
		variables = map[string]string{}
	}
	variables["STATION_API_BOOTSTRAP_URL"], variables["ADDONS"] = e.Station.BootstrapURL, e.AddonCatalog
	return &ComposeFile{File: tree.Compose, Variables: variables}
}

// Desired is the state the environment is to run with the release, in its clusters (cluster -> the Addons installed
// there, Site.Installed): every one of them runs the release's agents and Station, and the release's Addons where they
// are installed.
func (e Environment) Desired(r Release, clusters map[string][]string) State {
	images := r.Images
	state := State{
		ControlPlane:      map[string]string{},
		StationAPI:        images["station-api"],
		StationController: images["station-controller"],
		Installer:         images["installer-"+e.Name],
		Clusters:          map[string]ClusterState{},
		AddonsAfter:       map[string][]string{},
	}
	for service := range composeVariables {
		// An image as the control plane's machine names it.
		repository, digest, _ := strings.Cut(images[service], "@")
		if local, mapped := e.ControlPlane.Images[repository]; mapped {
			state.ControlPlane[service] = local + "@" + digest
		} else {
			state.ControlPlane[service] = images[service]
		}
	}
	for name, addon := range r.Addons {
		if len(addon.After) > 0 {
			state.AddonsAfter[name] = addon.After
		}
	}
	for name, installed := range clusters {
		desired := ClusterState{
			Agent: images["cluster-agent"], Tools: images["tools"], NodeAgent: images["node-agent"],
			StationAgent: images["station-agent"], StationWireguard: images["station-wireguard"], StationPullSecret: e.Station.PullSecret,
			Addons: map[string]AddonState{},
		}
		for _, addon := range installed {
			if brought, inRelease := r.Addons[addon]; inRelease {
				values, _ := r.render(brought.Values)
				desired.Addons[addon] = AddonState{Version: brought.Version, Values: values}
			}
		}
		state.Clusters[name] = desired
	}
	return state
}
