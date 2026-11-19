package addons

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.yaml.in/yaml/v3"
	"k8s.io/apimachinery/pkg/util/validation"
)

//go:embed catalog/*/addon.yaml
var addonYAMLs embed.FS

//go:embed catalog/*/manifests
var manifestsFS embed.FS

// Constants used by the package.
const (
	LabelAddonName = "rlark.io/addon-name"
	LabelAddonUID  = "rlark.io/addon-uid"
)

// ParameterType represents a parameter type.
type ParameterType string

// Constants used by the package.
const (
	ParamTypeString ParameterType = "string"
	ParamTypeText   ParameterType = "text"
	ParamTypeEnum   ParameterType = "enum"
	ParamTypeBool   ParameterType = "bool"
	ParamTypeInt    ParameterType = "int"
)

// AddonParameter describes an addon parameter.
type AddonParameter struct {
	Name        string        `yaml:"name" json:"name"`
	DisplayName string        `yaml:"displayName" json:"displayName"`
	Description string        `yaml:"description,omitempty" json:"description,omitempty"`
	Type        ParameterType `yaml:"type" json:"type"`
	Default     string        `yaml:"default,omitempty" json:"default,omitempty"`
	Options     []string      `yaml:"options,omitempty" json:"options,omitempty"`
	Required    bool          `yaml:"required,omitempty" json:"required,omitempty"`
	// RequiredWith names the parameter whose value makes this one required.
	RequiredWith string `yaml:"requiredWith,omitempty" json:"requiredWith,omitempty"`
}

// AddonMeta holds metadata.
type AddonMeta struct {
	Name        string           `yaml:"name" json:"name"`
	DisplayName string           `yaml:"displayName" json:"displayName"`
	Category    string           `yaml:"category" json:"category"`
	Version     string           `yaml:"version" json:"version"`
	Description string           `yaml:"description" json:"description"`
	Icon        string           `yaml:"icon,omitempty" json:"icon,omitempty"`
	Parameters  []AddonParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// Manifest describes an addon manifest.
type Manifest struct {
	Raw []byte
}

// Addon represents an addon.
type Addon interface {
	Meta() AddonMeta
	Render(values map[string]string, namespace string, addonName string, addonUID string) ([]Manifest, error)
}

type registry struct {
	addons map[string]Addon

	dir      string
	mu       sync.Mutex
	readAt   time.Time
	external []Addon
	problem  string
}

// Registry is an exported variable.
var Registry *registry

func init() {
	Registry = &registry{addons: make(map[string]Addon), dir: os.Getenv(CatalogDirEnv)}
	if err := Registry.load(); err != nil {
		panic(fmt.Sprintf("failed to load addon catalog: %v", err))
	}
}

func (r *registry) load() error {
	entries, err := addonYAMLs.ReadDir("catalog")
	if err != nil {
		return fmt.Errorf("read catalog dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		yamlPath := path.Join("catalog", entry.Name(), "addon.yaml")
		data, err := addonYAMLs.ReadFile(yamlPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", yamlPath, err)
		}

		var meta AddonMeta
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return fmt.Errorf("unmarshal %s: %w", yamlPath, err)
		}

		manifestsDir := path.Join("catalog", entry.Name(), "manifests")
		a, err := newAddon(meta, data, manifestsFS, manifestsDir)
		if err != nil {
			return err
		}
		r.addons[meta.Name] = a
	}

	return nil
}

// List returns every item of the catalog, each version of a name on its own.
func (r *registry) List() []AddonMeta {
	var result []AddonMeta
	for _, a := range r.addons {
		result = append(result, a.Meta())
	}
	for _, a := range r.fromDir() {
		result = append(result, a.Meta())
	}
	return result
}

// Get returns the current version of name: the built-in one, or for a name
// that is not built in the highest version the catalog directory holds.
func (r *registry) Get(name string) (Addon, bool) {
	if a, ok := r.addons[name]; ok {
		return a, true
	}
	var current Addon
	for _, a := range r.fromDir() {
		if a.Meta().Name == name && (current == nil || versionLess(current.Meta().Version, a.Meta().Version)) {
			current = a
		}
	}
	return current, current != nil
}

type embeddedAddon struct {
	meta AddonMeta
	// metaRaw is addon.yaml as written, templates the manifest templates in
	// the order of their names: together, the item's Package.
	metaRaw   []byte
	templates []manifestTemplate
}

// Meta is an exported method.
func (a *embeddedAddon) Meta() AddonMeta {
	return a.meta
}

// Render is an exported method.
func (a *embeddedAddon) Render(values map[string]string, namespace string, addonName string, addonUID string) ([]Manifest, error) {
	if a.meta.Name == "csi-driver-rclone" {
		for _, name := range []string{"nodeSelector", "controllerNodeSelector"} {
			value := values[name]
			if value == "" {
				continue
			}
			parts := strings.Split(value, "=")
			if len(parts) != 2 || len(validation.IsQualifiedName(parts[0])) != 0 || len(validation.IsValidLabelValue(parts[1])) != 0 {
				return nil, fmt.Errorf("parameter %s must be a valid label=value selector", name)
			}
		}
	}
	renderValues := make(map[string]interface{})
	for _, p := range a.meta.Parameters {
		v, ok := values[p.Name]
		if !ok {
			v = p.Default
		}
		if v == "" && p.Required {
			return nil, fmt.Errorf("parameter %s is required", p.Name)
		}
		renderValues[p.Name] = v
		if a.meta.Name == "csi-driver-rclone" && (p.Name == "nodeUpdateStrategy" || p.Name == "nodeImagePullPolicy") {
			valid := false
			for _, option := range p.Options {
				valid = valid || v == option
			}
			if !valid {
				return nil, fmt.Errorf("parameter %s must be one of %v", p.Name, p.Options)
			}
		}
	}

	addonLabels := map[string]string{
		LabelAddonName: addonName,
		LabelAddonUID:  addonUID,
	}

	data := struct {
		Values      map[string]interface{}
		Namespace   string
		AddonName   string
		AddonUID    string
		AddonLabels map[string]string
	}{
		Values:      renderValues,
		Namespace:   namespace,
		AddonName:   addonName,
		AddonUID:    addonUID,
		AddonLabels: addonLabels,
	}

	var result []Manifest
	for _, manifest := range a.templates {
		fname, raw := manifest.name, manifest.raw
		tmpl, err := template.New(fname).Funcs(template.FuncMap{
			"splitList": func(sep, s string) []string {
				if s == "" {
					return nil
				}
				return strings.Split(s, sep)
			},
			"indent": func(n int, s string) string {
				prefix := strings.Repeat(" ", n)
				return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
			},
		}).Parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", fname, err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("execute template %s: %w", fname, err)
		}

		result = append(result, Manifest{Raw: buf.Bytes()})
	}

	return result, nil
}
