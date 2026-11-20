package addons

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"strings"
	"text/template"

	"go.yaml.in/yaml/v3"
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
}

// Registry is an exported variable.
var Registry *registry

func init() {
	Registry = &registry{addons: make(map[string]Addon)}
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
		manifestEntries, err := manifestsFS.ReadDir(manifestsDir)
		if err != nil {
			return fmt.Errorf("read manifests dir %s: %w", manifestsDir, err)
		}

		var manifestFiles []string
		for _, me := range manifestEntries {
			if me.IsDir() || path.Ext(me.Name()) != ".yaml" {
				continue
			}
			manifestFiles = append(manifestFiles, me.Name())
		}

		a := &embeddedAddon{
			meta:          meta,
			dir:           entry.Name(),
			manifestFiles: manifestFiles,
		}
		r.addons[meta.Name] = a
	}

	return nil
}

// List is an exported method.
func (r *registry) List() []AddonMeta {
	var result []AddonMeta
	for _, a := range r.addons {
		result = append(result, a.Meta())
	}
	return result
}

// Get is an exported method.
func (r *registry) Get(name string) (Addon, bool) {
	a, ok := r.addons[name]
	return a, ok
}

type embeddedAddon struct {
	meta          AddonMeta
	dir           string
	manifestFiles []string
}

// Meta is an exported method.
func (a *embeddedAddon) Meta() AddonMeta {
	return a.meta
}

// Render is an exported method.
func (a *embeddedAddon) Render(values map[string]string, namespace string, addonName string, addonUID string) ([]Manifest, error) {
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
	for _, fname := range a.manifestFiles {
		manifestPath := path.Join("catalog", a.dir, "manifests", fname)
		raw, err := manifestsFS.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", manifestPath, err)
		}

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
