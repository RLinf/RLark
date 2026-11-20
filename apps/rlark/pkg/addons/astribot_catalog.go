package addons

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.yaml.in/yaml/v3"
	"k8s.io/apimachinery/pkg/util/validation"
	utilversion "k8s.io/apimachinery/pkg/util/version"
)

//go:embed catalog/*/bootstrap
var publishedFS embed.FS

// Published returns a file a built-in item publishes for machines that are
// not nodes yet (its bootstrap directory; a NodeClass's uplink.setup names
// one), named relative to the item, and whether the item is built in. What
// an item of the catalog directory publishes is read there by its reader.
func Published(item, name string) (data []byte, builtIn bool, err error) {
	if _, builtIn = Catalog.builtIn[item]; !builtIn {
		return nil, false, nil
	}
	if !fs.ValidPath(name) || !strings.HasPrefix(name, "bootstrap/") {
		return nil, true, fmt.Errorf("%s of %s is not a file the item publishes", name, item)
	}
	data, err = publishedFS.ReadFile(path.Join("catalog", item, name))
	return data, true, err
}

// CatalogDirEnv names a directory of catalog items that are not compiled in.
// Each item is a subdirectory holding addon.yaml and its manifest templates,
// either in manifests/ or beside addon.yaml (the shape of a mounted
// ConfigMap, whose keys cannot contain a slash). The directory is read again
// when it is asked for and the last reading is older than a few seconds, so
// an item added to it is offered without rebuilding or restarting anything.
//
// An item is a name at a version. The directory may hold further versions of
// a built-in item, and several of one name; what it may not hold is other
// content for a name and version that is built in.
//
// The catalog is the control plane's. A cluster's agent renders an Addon from
// the Package of its name and version that the control plane published to
// the cluster (see Package), not from a catalog of its own.
const CatalogDirEnv = "RLARK_ADDON_CATALOG_DIR"

// ItemParameter is a parameter of a catalog item: the official description and what our catalog adds to it.
type ItemParameter struct {
	AddonParameter `yaml:",inline"`
	// RequiredWith names the parameter whose value makes this one required.
	RequiredWith string `yaml:"requiredWith,omitempty" json:"requiredWith,omitempty"`
}

// ItemMeta is an item's addon.yaml: the official AddonMeta, and its parameters again as ours (the ones the JSON
// shows and Render reads).
type ItemMeta struct {
	AddonMeta
	Parameters []ItemParameter `json:"parameters,omitempty"`
}

func parseMeta(raw []byte) (ItemMeta, error) {
	var meta ItemMeta
	if err := yaml.Unmarshal(raw, &meta.AddonMeta); err != nil {
		return meta, err
	}
	err := yaml.Unmarshal(raw, &struct{ Parameters *[]ItemParameter }{&meta.Parameters})
	return meta, err
}

// Item is a catalog item at one version, built in, of the catalog directory or read back from a published Package.
// The official embeddedAddon renders only what is compiled in, from the files of the build; an Item carries its own.
type Item struct {
	meta ItemMeta
	// metaRaw is addon.yaml as written, templates the manifest templates in
	// the order of their names: together, the item's Package.
	metaRaw   []byte
	templates []manifestTemplate
}

type manifestTemplate struct {
	name string
	raw  []byte
}

// Meta returns the item's addon.yaml.
func (a *Item) Meta() ItemMeta {
	return a.meta
}

// Render renders the item's manifests as the official embeddedAddon.Render does its own, and checks the values of
// csi-driver-rclone that end in a selector or in a field with fixed options. The official one reads only the
// files of the build by directory, so the two cannot share code: TestItemsRenderAsTheOfficialRegistryDoes holds
// this one to it.
func (a *Item) Render(values map[string]string, namespace string, addonName string, addonUID string) ([]Manifest, error) {
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
		AddonLabels: map[string]string{LabelAddonName: addonName, LabelAddonUID: addonUID},
	}

	var result []Manifest
	for _, manifest := range a.templates {
		tmpl, err := template.New(manifest.name).Funcs(template.FuncMap{
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
		}).Parse(string(manifest.raw))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", manifest.name, err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("execute template %s: %w", manifest.name, err)
		}
		result = append(result, Manifest{Raw: buf.Bytes()})
	}
	return result, nil
}

// readTemplates reads an item's manifest templates (the .yaml files of dir in files, other than addon.yaml).
func readTemplates(files fs.FS, dir string) ([]manifestTemplate, error) {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil, fmt.Errorf("read manifests dir %s: %w", dir, err)
	}
	var templates []manifestTemplate
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == packageMetaFile || strings.HasPrefix(name, "..") || path.Ext(name) != ".yaml" {
			continue
		}
		raw, err := fs.ReadFile(files, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", path.Join(dir, name), err)
		}
		templates = append(templates, manifestTemplate{name: name, raw: raw})
	}
	sortTemplates(templates)
	return templates, nil
}

func sortTemplates(templates []manifestTemplate) {
	sort.Slice(templates, func(i, j int) bool { return templates[i].name < templates[j].name })
}

// catalog is what the control plane offers: the items compiled in (the ones the official Registry holds) and
// those of the catalog directory. The official Registry stays as it is and is not asked.
type catalog struct {
	builtIn map[string]*Item

	dir      string
	mu       sync.Mutex
	readAt   time.Time
	external []*Item
	problem  string
}

// Catalog is the catalog of this program.
var Catalog = newCatalog(os.Getenv(CatalogDirEnv))

func newCatalog(dir string) *catalog {
	c := &catalog{builtIn: map[string]*Item{}, dir: dir}
	entries, err := addonYAMLs.ReadDir("catalog")
	if err != nil {
		panic(fmt.Sprintf("failed to load addon catalog: %v", err))
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		yamlPath := path.Join("catalog", entry.Name(), packageMetaFile)
		data, err := addonYAMLs.ReadFile(yamlPath)
		if err != nil {
			panic(fmt.Sprintf("failed to load addon catalog: read %s: %v", yamlPath, err))
		}
		meta, err := parseMeta(data)
		if err != nil {
			panic(fmt.Sprintf("failed to load addon catalog: unmarshal %s: %v", yamlPath, err))
		}
		templates, err := readTemplates(manifestsFS, path.Join("catalog", entry.Name(), "manifests"))
		if err != nil {
			panic(fmt.Sprintf("failed to load addon catalog: %v", err))
		}
		c.builtIn[meta.Name] = &Item{meta: meta, metaRaw: data, templates: templates}
	}
	return c
}

// List returns every item of the catalog, each version of a name on its own.
func (r *catalog) List() []ItemMeta {
	var result []ItemMeta
	for _, a := range r.builtIn {
		result = append(result, a.Meta())
	}
	for _, a := range r.fromDir() {
		result = append(result, a.Meta())
	}
	return result
}

// Get returns the current version of name: the built-in one, or for a name
// that is not built in the highest version the catalog directory holds.
func (r *catalog) Get(name string) (*Item, bool) {
	if a, ok := r.builtIn[name]; ok {
		return a, true
	}
	var current *Item
	for _, a := range r.fromDir() {
		if a.meta.Name == name && (current == nil || versionLess(current.meta.Version, a.meta.Version)) {
			current = a
		}
	}
	return current, current != nil
}

// SetDirectory points the catalog at a directory of items that are not compiled in, as CatalogDirEnv does,
// for a program given it otherwise than by its environment (rlarkctl release: the environment file's).
func (r *catalog) SetDirectory(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dir, r.readAt = dir, time.Time{}
}

// fromDir reads the items of the catalog directory, at most every five seconds.
func (r *catalog) fromDir() []*Item {
	if r.dir == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.readAt) < 5*time.Second {
		return r.external
	}
	r.readAt = time.Now()
	var found []*Item
	var problems []string
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		problems = append(problems, err.Error())
	}
	for _, entry := range entries {
		// A mounted ConfigMap keeps its revisions in entries that start with "..".
		if strings.HasPrefix(entry.Name(), "..") {
			continue
		}
		item := os.DirFS(path.Join(r.dir, entry.Name()))
		data, err := fs.ReadFile(item, packageMetaFile)
		if err != nil {
			continue
		}
		meta, err := parseMeta(data)
		if err != nil || meta.Name == "" {
			problems = append(problems, fmt.Sprintf("%s/addon.yaml is not a catalog item: %v", entry.Name(), err))
			continue
		}
		if builtIn, is := r.builtIn[meta.Name]; is && builtIn.meta.Version == meta.Version {
			problems = append(problems, fmt.Sprintf("%s: %s %s is built in and is not replaced", entry.Name(), meta.Name, meta.Version))
			continue
		}
		manifestsDir := "manifests"
		if _, err := fs.ReadDir(item, manifestsDir); err != nil {
			manifestsDir = "."
		}
		templates, err := readTemplates(item, manifestsDir)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		found = append(found, &Item{meta: meta, metaRaw: data, templates: templates})
	}
	if problem := strings.Join(problems, "; "); problem != r.problem {
		r.problem = problem
		if problem != "" {
			log.Printf("addon catalog directory %s: %s", r.dir, problem)
		}
	}
	r.external = found
	return found
}

// GetVersion returns name at version; an empty version is the current one.
func (r *catalog) GetVersion(name, version string) (*Item, bool) {
	if version == "" {
		return r.Get(name)
	}
	if a, ok := r.builtIn[name]; ok && a.meta.Version == version {
		return a, true
	}
	for _, a := range r.fromDir() {
		if a.meta.Name == name && a.meta.Version == version {
			return a, true
		}
	}
	return nil, false
}

// versionLess orders versions as numbers where both read as such (v0.9.3 before v0.10.0), else as text.
func versionLess(a, b string) bool {
	left, errLeft := utilversion.ParseGeneric(a)
	right, errRight := utilversion.ParseGeneric(b)
	if errLeft != nil || errRight != nil {
		return a < b
	}
	return left.LessThan(right)
}
