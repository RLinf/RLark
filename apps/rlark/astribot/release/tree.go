package release

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/rlinf/rlark/apps/rlark/astribot/identity"
)

// definitionFiles are the files of the tree whose objects are the
// management plane's definitions: the CRDs and Station's management RBAC.
var definitionFiles = []string{
	"api/config/crd/bases/*.yaml",
	"astribot/station/config/crd/bases/*.yaml",
	"astribot/station/deploy/management-rbac.yaml",
	"astribot/station/deploy/node-operation-management-rbac.yaml",
}

// memberDefinitionFiles are the files of the tree whose objects every member
// cluster holds: what Station API and Station's node operations may do there
// (each cluster runs Station), and what the release itself may.
var memberDefinitionFiles = []string{
	"astribot/station/deploy/member-rbac.yaml",
	"astribot/station/deploy/node-operation-member-rbac.yaml",
	releaseMemberRBAC,
}

// releaseMemberRBAC is what the release may do in a member cluster (IssueMemberCredential).
const releaseMemberRBAC = "astribot/release/member-rbac.yaml"

// recordedFiles are the tree's files the record of the release a site runs carries as they are (CurrentRelease), by
// the key they have there: what a cluster made after the release is given (the Gateway's image has no tree).
var recordedFiles = map[string]string{
	"astribot/station/deploy/member-rbac.yaml":                "member-rbac.yaml",
	"astribot/station/deploy/node-operation-member-rbac.yaml": "node-operation-member-rbac.yaml",
	"astribot/station/deploy/station-agent.yaml":              "station-agent.yaml",
	releaseMemberRBAC: "release-member-rbac.yaml",
}

// definitionKinds is where each kind of definition is written.
var definitionKinds = map[string]struct {
	resource   schema.GroupVersionResource
	namespaced bool
}{
	"CustomResourceDefinition": {schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}, false},
	"ClusterRole":              {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, false},
	"ClusterRoleBinding":       {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, false},
	"Role":                     {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, true},
	"RoleBinding":              {schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, true},
	"ServiceAccount":           {schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}, true},
	"Namespace":                {schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, false},
	// A ServiceAccount's token (the release's own in a member cluster).
	"Secret": {schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, true},
}

// Tree is the source of a release as `git archive <commit>` writes it: the commit is in the archive's own header, so
// the files are known to be the release's and nobody else's.
type Tree struct {
	Commit      string
	Definitions []*unstructured.Unstructured
	// MemberDefinitions are written into every member cluster.
	MemberDefinitions []*unstructured.Unstructured
	// AddonVersions is the version of every Addon of the tree's catalog.
	AddonVersions map[string]string
	// Files are the recordedFiles, by their key in the record.
	Files map[string]string
	// Pinned is what a release of the tree brings besides its built images (astribot/release/pinned.yaml).
	Pinned Pinned
	// Compose is the control plane's compose file (astribot/release/control-plane/rlark.compose.yaml).
	Compose string
	// Environments are the names of the tree's environments (astribot/release/environments/<name>.yaml).
	Environments []string
}

// Pinned are images no build of a tree makes (component -> image by digest) and values of the catalog's Addons that
// are the same in every cluster; a release of the tree states them (Candidate).
type Pinned struct {
	Images map[string]string     `json:"images"`
	Addons map[string]AddonState `json:"addons"`
	// Installer are the components (of Images) whose images every node has from its installer, by their names.
	Installer []string `json:"installer"`
}

const (
	catalogFiles     = "apps/rlark/pkg/addons/catalog/*/addon.yaml"
	pinnedFile       = "astribot/release/pinned.yaml"
	composeFile      = "astribot/release/control-plane/rlark.compose.yaml"
	environmentFiles = "astribot/release/environments/*.yaml"
)

// ReadTree reads what a release takes of its tree out of an archive of it.
func ReadTree(file string) (Tree, error) {
	f, err := os.Open(file)
	if err != nil {
		return Tree{}, err
	}
	defer f.Close()
	tree := Tree{AddonVersions: map[string]string{}, Files: map[string]string{}}
	reader := tar.NewReader(f)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Tree{}, fmt.Errorf("%s: %w", file, err)
		}
		name := header.Name
		matches := func(pattern string) bool { matched, _ := path.Match(pattern, name); return matched }
		if header.Typeflag == tar.TypeXGlobalHeader {
			tree.Commit = header.PAXRecords["comment"]
			continue
		}
		if matches(environmentFiles) {
			tree.Environments = append(tree.Environments, strings.TrimSuffix(path.Base(name), ".yaml"))
			continue
		}
		catalog := matches(catalogFiles)
		definitions, memberDefinitions := slices.ContainsFunc(definitionFiles, matches), slices.ContainsFunc(memberDefinitionFiles, matches)
		recorded, isRecorded := recordedFiles[name]
		if !catalog && !definitions && !memberDefinitions && !isRecorded && name != pinnedFile && name != composeFile {
			continue
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return Tree{}, err
		}
		if isRecorded {
			tree.Files[recorded] = string(data)
		}
		switch {
		case name == composeFile:
			tree.Compose = string(data)
		case catalog:
			var item struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if err := yaml.Unmarshal(data, &item); err != nil {
				return Tree{}, fmt.Errorf("%s: %w", name, err)
			}
			tree.AddonVersions[item.Name] = item.Version
		case name == pinnedFile:
			if err := yaml.UnmarshalStrict(data, &tree.Pinned); err != nil {
				return Tree{}, fmt.Errorf("%s: %w", name, err)
			}
			for component, image := range tree.Pinned.Images {
				if !strings.Contains(image, "@sha256:") {
					return Tree{}, fmt.Errorf("%s: image %s is not named by its digest", name, component)
				}
			}
		case definitions || memberDefinitions:
			parsed, err := ParseDefinitions(name, data)
			if err != nil {
				return Tree{}, err
			}
			if memberDefinitions {
				tree.MemberDefinitions = append(tree.MemberDefinitions, parsed...)
			} else {
				tree.Definitions = append(tree.Definitions, parsed...)
			}
		}
	}
	if tree.Commit == "" {
		return Tree{}, fmt.Errorf("%s has no commit in its header: it is not what `git archive <commit>` writes", file)
	}
	identities, err := identityDefinitions()
	tree.Definitions = append(tree.Definitions, identities...)
	return tree, err
}

// ParseDefinitions reads the objects of a file of definitions (YAML documents), each of a kind the release writes.
func ParseDefinitions(name string, data []byte) ([]*unstructured.Unstructured, error) {
	var definitions []*unstructured.Unstructured
	for _, document := range bytes.Split(data, []byte("\n---")) {
		var object map[string]any
		if err := yaml.Unmarshal(document, &object); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if object == nil {
			continue
		}
		definition := &unstructured.Unstructured{Object: object}
		if _, known := definitionKinds[definition.GetKind()]; !known {
			return nil, fmt.Errorf("%s: a %s is not a definition the release writes", name, definition.GetKind())
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func definitionClient(management dynamic.Interface, definition *unstructured.Unstructured) dynamic.ResourceInterface {
	kind := definitionKinds[definition.GetKind()]
	if kind.namespaced {
		return management.Resource(kind.resource).Namespace(definition.GetNamespace())
	}
	return management.Resource(kind.resource)
}

func definitionName(definition *unstructured.Unstructured) string {
	return strings.Trim(definition.GetKind()+" "+definition.GetNamespace()+"/"+definition.GetName(), "/")
}

// definitionChanges are the definitions a plane (the management plane, or
// the member cluster where) does not hold as the tree states them: absent,
// or a field of what the tree says differs.
func definitionChanges(ctx context.Context, plane dynamic.Interface, definitions []*unstructured.Unstructured, where, commit string) ([]Change, error) {
	var out []Change
	for _, definition := range definitions {
		what := strings.TrimSpace(where + " " + definitionName(definition))
		current, err := definitionClient(plane, definition).Get(ctx, definition.GetName(), metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			out = append(out, Change{Stage: "crds-rbac", What: what, From: "", To: "as in " + short(commit)})
			continue
		case err != nil:
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		if differs := differences(jsonValue(current.Object), jsonValue(stated(definition)), false); len(differs) > 0 {
			var at []string
			for _, differs := range differs[:min(3, len(differs))] {
				at = append(at, fmt.Sprintf("%s: %s, %s stated", differs.path, short(differs.from), short(differs.to)))
			}
			out = append(out, Change{Stage: "crds-rbac", What: what, From: "differs at " + strings.Join(at, "; "), To: "as in " + short(commit)})
		}
	}
	return out, nil
}

// stated is what a definition states beyond its metadata and status.
func stated(definition *unstructured.Unstructured) map[string]any {
	want := map[string]any{}
	for key, value := range definition.Object {
		if key != "metadata" && key != "status" {
			want[key] = value
		}
	}
	return want
}

func applyDefinitions(ctx context.Context, plane dynamic.Interface, definitions []*unstructured.Unstructured, where string) error {
	for _, definition := range definitions {
		data, err := json.Marshal(definition.Object)
		if err != nil {
			return err
		}
		force := true
		if _, err := definitionClient(plane, definition).Patch(ctx, definition.GetName(), types.ApplyPatchType, data,
			metav1.PatchOptions{FieldManager: "rlark-release", Force: &force}); err != nil {
			return fmt.Errorf("%s: %w", strings.TrimSpace(where+" "+definitionName(definition)), err)
		}
	}
	return nil
}

// Candidate is the release of a tree: base with what the tree pins and the images built from it (component ->
// image), the tree's Addon versions and the commits it is built from. It is written beside base.
func Candidate(base Release, tree Tree, name, fleet string, built map[string]string) (Release, error) {
	candidate := base
	candidate.Name, candidate.Source.RLark, candidate.Source.Fleet = name, tree.Commit, fleet
	candidate.Images = maps.Clone(base.Images)
	maps.Copy(candidate.Images, tree.Pinned.Images)
	maps.Copy(candidate.Images, built)
	// The base's Addons and the tree's pinned ones, at the versions of the tree's catalog; a value pinned is the tree's.
	candidate.Addons = map[string]AddonState{}
	for addon, brought := range base.Addons {
		brought.Values = maps.Clone(brought.Values)
		candidate.Addons[addon] = brought
	}
	for addon, pinned := range tree.Pinned.Addons {
		brought := candidate.Addons[addon]
		if brought.Values == nil {
			brought.Values = map[string]string{}
		}
		maps.Copy(brought.Values, pinned.Values)
		if len(pinned.After) > 0 {
			brought.After = pinned.After
		}
		candidate.Addons[addon] = brought
	}
	// An image of the base nothing names any more (of what the tree dropped) is not carried on: the candidate's are
	// the components, what the tree pins and was built of it, the installers of the tree's environments
	// (installer-<environment>), and what its Addons' values name.
	named := map[string]bool{}
	for _, component := range slices.Concat(Components, slices.Collect(maps.Keys(tree.Pinned.Images)), slices.Collect(maps.Keys(built))) {
		named[component] = true
	}
	for _, environment := range tree.Environments {
		named["installer-"+environment] = true
	}
	for addon, brought := range candidate.Addons {
		version, found := tree.AddonVersions[addon]
		if !found {
			return Release{}, fmt.Errorf("Addon %s of release %s is not in the catalog of %s", addon, candidate.Name, tree.Commit)
		}
		brought.Version = version
		candidate.Addons[addon] = brought
		for _, value := range brought.Values {
			for _, match := range reference.FindAllStringSubmatch(value, -1) {
				named[match[1]] = true
			}
		}
	}
	maps.DeleteFunc(candidate.Images, func(component, _ string) bool { return !named[component] })
	return candidate, nil
}

// Write writes the release as YAML into the directory of the release it was made from.
func (r Release) Write() (string, error) {
	data, err := yaml.Marshal(r)
	if err != nil {
		return "", err
	}
	path := filepath.Join(r.dir, r.Name+".yaml")
	return path, os.WriteFile(path, data, 0o644)
}

// identityDefinitions are what each of RLark's own clients of the management plane may do there, as this release's
// rlarkadm states it (identity.RLark): the ClusterRole of its rules and its binding. The release writes them with the
// CRDs, so a component that comes with new permissions has them before it starts (11-17: a Gateway that makes
// clusters, without its role for clusterprovisions). The ServiceAccount and its token are rlarkadm identity's, which
// a release does not touch; Station's are in its management RBAC.
func identityDefinitions() ([]*unstructured.Unstructured, error) {
	var out []*unstructured.Unstructured
	add := func(object runtime.Object) error {
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return err
		}
		out = append(out, &unstructured.Unstructured{Object: data})
		return nil
	}
	for _, c := range identity.RLark() {
		role := c.ClusterRole
		if c.Rules != nil {
			role = c.Name
			if err := add(&rbacv1.ClusterRole{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole"},
				ObjectMeta: metav1.ObjectMeta{Name: role}, Rules: c.Rules,
			}); err != nil {
				return nil, err
			}
		}
		if err := add(&rbacv1.ClusterRoleBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: c.Name},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: c.ServiceAccount, Namespace: c.Namespace}},
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}
