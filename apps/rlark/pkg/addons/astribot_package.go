package addons

import (
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A Package is one catalog item at one version, as files: addon.yaml and its
// manifest templates. The control plane publishes the Package of every Addon
// it installs or updates to the Addon's cluster, as an immutable ConfigMap in
// the cluster's management namespace; the cluster's agent renders the Addon
// from it. So what an Addon's workloads are made from is the Addon's own name
// and version, whatever build the agent or the control plane is: replacing
// either changes no Addon. A name at a version has one content for ever; other
// content is another version.
type Package struct {
	Name    string
	Version string
	Files   map[string]string
}

const (
	packageMetaFile = "addon.yaml"
	// LabelPackage marks a published Package and names its item.
	LabelPackage = "rlark.io/addon-package"
)

var notNameCharacters = regexp.MustCompile(`[^a-z0-9.-]+`)

// PackageName names the ConfigMap that holds name at version.
func PackageName(name, version string) string {
	return "addon-package-" + name + "-" + strings.Trim(notNameCharacters.ReplaceAllString(strings.ToLower(version), "-"), "-.")
}

// PackageOf returns the Package of a catalog item.
func PackageOf(a Addon) Package {
	item := a.(*embeddedAddon)
	files := map[string]string{packageMetaFile: string(item.metaRaw)}
	for _, manifest := range item.templates {
		files[manifest.name] = string(manifest.raw)
	}
	return Package{Name: item.meta.Name, Version: item.meta.Version, Files: files}
}

// ConfigMap is the Package as it is published to a cluster's management namespace.
func (p Package) ConfigMap(namespace string) *corev1.ConfigMap {
	immutable := true
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: PackageName(p.Name, p.Version), Namespace: namespace, Labels: map[string]string{LabelPackage: p.Name}},
		Immutable:  &immutable,
		Data:       p.Files,
	}
}

// FromPackage returns the item a published Package holds, to be rendered.
func FromPackage(files map[string]string) (Addon, error) {
	raw, found := files[packageMetaFile]
	if !found {
		return nil, fmt.Errorf("the package has no %s", packageMetaFile)
	}
	var meta AddonMeta
	if err := yaml.Unmarshal([]byte(raw), &meta); err != nil || meta.Name == "" {
		return nil, fmt.Errorf("the package's %s is not a catalog item: %v", packageMetaFile, err)
	}
	a := &embeddedAddon{meta: meta, metaRaw: []byte(raw)}
	for name, content := range files {
		if name != packageMetaFile {
			a.templates = append(a.templates, manifestTemplate{name: name, raw: []byte(content)})
		}
	}
	sortTemplates(a.templates)
	return a, nil
}

// MissingRequired names the parameters of the item that are required, or
// required with another that has a value, and that values and the defaults
// leave empty.
func MissingRequired(meta AddonMeta, values map[string]string) []string {
	value := map[string]string{}
	for _, parameter := range meta.Parameters {
		given, ok := values[parameter.Name]
		if !ok {
			given = parameter.Default
		}
		value[parameter.Name] = given
	}
	var missing []string
	for _, parameter := range meta.Parameters {
		if value[parameter.Name] == "" && (parameter.Required || parameter.RequiredWith != "" && value[parameter.RequiredWith] != "") {
			missing = append(missing, parameter.Name)
		}
	}
	return missing
}
