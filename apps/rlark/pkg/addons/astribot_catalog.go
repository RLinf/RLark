package addons

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	utilversion "k8s.io/apimachinery/pkg/util/version"
)

//go:embed catalog/*/bootstrap
var publishedFS embed.FS

// Published returns a file a built-in item publishes for machines that are
// not nodes yet (its bootstrap directory; a NodeClass's uplink.setup names
// one), named relative to the item, and whether the item is built in. What
// an item of the catalog directory publishes is read there by its reader.
func Published(item, name string) (data []byte, builtIn bool, err error) {
	if _, builtIn = Registry.addons[item]; !builtIn {
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

// newAddon reads an item's manifest templates (the .yaml files of dir in files, other than addon.yaml).
func newAddon(meta AddonMeta, metaRaw []byte, files fs.FS, dir string) (*embeddedAddon, error) {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil, fmt.Errorf("read manifests dir %s: %w", dir, err)
	}
	a := &embeddedAddon{meta: meta, metaRaw: metaRaw}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == packageMetaFile || strings.HasPrefix(name, "..") || path.Ext(name) != ".yaml" {
			continue
		}
		raw, err := fs.ReadFile(files, path.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read manifest %s: %w", path.Join(dir, name), err)
		}
		a.templates = append(a.templates, manifestTemplate{name: name, raw: raw})
	}
	sortTemplates(a.templates)
	return a, nil
}

func sortTemplates(templates []manifestTemplate) {
	sort.Slice(templates, func(i, j int) bool { return templates[i].name < templates[j].name })
}

// SetDirectory points the catalog at a directory of items that are not compiled in, as CatalogDirEnv does,
// for a program given it otherwise than by its environment (rlarkctl release: the environment file's).
func (r *registry) SetDirectory(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dir, r.readAt = dir, time.Time{}
}

// fromDir reads the items of the catalog directory, at most every five seconds.
func (r *registry) fromDir() []Addon {
	if r.dir == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.readAt) < 5*time.Second {
		return r.external
	}
	r.readAt = time.Now()
	var found []Addon
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
		var meta AddonMeta
		if err := yaml.Unmarshal(data, &meta); err != nil || meta.Name == "" {
			problems = append(problems, fmt.Sprintf("%s/addon.yaml is not a catalog item: %v", entry.Name(), err))
			continue
		}
		if builtIn, is := r.addons[meta.Name]; is && builtIn.Meta().Version == meta.Version {
			problems = append(problems, fmt.Sprintf("%s: %s %s is built in and is not replaced", entry.Name(), meta.Name, meta.Version))
			continue
		}
		manifestsDir := "manifests"
		if _, err := fs.ReadDir(item, manifestsDir); err != nil {
			manifestsDir = "."
		}
		a, err := newAddon(meta, data, item, manifestsDir)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		found = append(found, a)
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
func (r *registry) GetVersion(name, version string) (Addon, bool) {
	if version == "" {
		return r.Get(name)
	}
	if a, ok := r.addons[name]; ok && a.Meta().Version == version {
		return a, true
	}
	for _, a := range r.fromDir() {
		if a.Meta().Name == name && a.Meta().Version == version {
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

type manifestTemplate struct {
	name string
	raw  []byte
}
