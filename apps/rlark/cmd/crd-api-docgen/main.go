package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v2"
)

type crdDocument struct {
	APIVersion string  `yaml:"apiVersion"`
	Kind       string  `yaml:"kind"`
	Spec       crdSpec `yaml:"spec"`
}

type crdSpec struct {
	Group    string       `yaml:"group"`
	Names    crdNames     `yaml:"names"`
	Scope    string       `yaml:"scope"`
	Versions []crdVersion `yaml:"versions"`
}

type crdNames struct {
	Kind       string   `yaml:"kind"`
	Plural     string   `yaml:"plural"`
	Singular   string   `yaml:"singular"`
	ShortNames []string `yaml:"shortNames"`
	ListKind   string   `yaml:"listKind"`
}

type crdVersion struct {
	Name         string          `yaml:"name"`
	Served       bool            `yaml:"served"`
	Storage      bool            `yaml:"storage"`
	Schema       crdSchema       `yaml:"schema"`
	Subresources crdSubresources `yaml:"subresources"`
}

type crdSchema struct {
	OpenAPIV3Schema schemaNode `yaml:"openAPIV3Schema"`
}

type crdSubresources struct {
	Status map[string]any `yaml:"status"`
}

type schemaNode struct {
	Type                 string                `yaml:"type"`
	Description          string                `yaml:"description"`
	Format               string                `yaml:"format"`
	Properties           map[string]schemaNode `yaml:"properties"`
	Items                *schemaNode           `yaml:"items"`
	Required             []string              `yaml:"required"`
	Enum                 []any                 `yaml:"enum"`
	AdditionalProperties *schemaNode           `yaml:"additionalProperties"`
	XIntOrString         bool                  `yaml:"x-kubernetes-int-or-string"`
}

type apiOperation struct {
	Method      string
	Path        string
	Description string
	Params      []string
	Body        string
	Responses   []apiResponse
}

type apiResponse struct {
	Code   int
	Desc   string
	Schema string
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <crd-dir> <output-file>\n", os.Args[0])
		os.Exit(2)
	}

	crdDir := os.Args[1]
	outputPath := os.Args[2]

	paths, err := filepath.Glob(filepath.Join(crdDir, "*.yaml"))
	if err != nil {
		fail(err)
	}
	if len(paths) == 0 {
		fail(fmt.Errorf("no CRD files found in %s", crdDir))
	}

	var docs []crdDocument
	for _, path := range paths {
		parsed, err := loadCRDs(path)
		if err != nil {
			fail(fmt.Errorf("load %s: %w", path, err))
		}
		docs = append(docs, parsed...)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Spec.Names.Kind < docs[j].Spec.Names.Kind
	})

	var out bytes.Buffer
	_, _ = fmt.Fprintf(&out, "# CRD Schema Reference\n\n")
	_, _ = fmt.Fprintf(&out, "Kubernetes resource operations and schemas generated from the current CRD manifests. This is not the RLark Gateway HTTP API reference.\n\n")
	for _, doc := range docs {
		if doc.Kind != "CustomResourceDefinition" {
			continue
		}
		version, ok := storageVersion(doc.Spec.Versions)
		if !ok {
			continue
		}
		writeResourceSection(&out, doc, version)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fail(err)
	}
	output := append(bytes.TrimSpace(out.Bytes()), '\n')
	if err := os.WriteFile(outputPath, output, 0o644); err != nil {
		fail(err)
	}
}

func loadCRDs(path string) ([]crdDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var docs []crdDocument
	for {
		var doc crdDocument
		if err := decoder.Decode(&doc); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		if strings.TrimSpace(doc.Kind) == "" {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func storageVersion(versions []crdVersion) (crdVersion, bool) {
	for _, version := range versions {
		if version.Storage || version.Served {
			return version, true
		}
	}
	return crdVersion{}, false
}

func writeResourceSection(out *bytes.Buffer, doc crdDocument, version crdVersion) {
	kind := doc.Spec.Names.Kind
	group := doc.Spec.Group
	plural := doc.Spec.Names.Plural
	listKind := doc.Spec.Names.ListKind
	if listKind == "" {
		listKind = kind + "List"
	}
	versionName := version.Name
	basePath := resourceBasePath(doc.Spec.Scope, group, versionName, plural)

	_, _ = fmt.Fprintf(out, "## %s\n\n", kind)
	_, _ = fmt.Fprintf(out, "- Group: `%s`\n", group)
	_, _ = fmt.Fprintf(out, "- Version: `%s`\n", versionName)
	_, _ = fmt.Fprintf(out, "- Scope: `%s`\n", doc.Spec.Scope)
	_, _ = fmt.Fprintf(out, "- Resource: `%s`\n\n", plural)

	_, _ = fmt.Fprintf(out, "### Operations\n\n")
	for _, op := range buildOperations(kind, plural, listKind, doc.Spec.Scope, basePath, version.Subresources.Status != nil) {
		_, _ = fmt.Fprintf(out, "#### `%s %s`\n\n", op.Method, op.Path)
		_, _ = fmt.Fprintf(out, "%s\n\n", op.Description)
		_, _ = fmt.Fprintf(out, "Parameters:\n")
		for _, param := range op.Params {
			_, _ = fmt.Fprintf(out, "- %s\n", param)
		}
		if op.Body != "" {
			_, _ = fmt.Fprintf(out, "\nRequest body: `%s`\n", op.Body)
		}
		_, _ = fmt.Fprintf(out, "\nResponses:\n")
		for _, resp := range op.Responses {
			_, _ = fmt.Fprintf(out, "- `%d` %s", resp.Code, resp.Desc)
			if resp.Schema != "" {
				_, _ = fmt.Fprintf(out, " → `%s`", resp.Schema)
			}
			_, _ = fmt.Fprintf(out, "\n")
		}
		_, _ = fmt.Fprintf(out, "\n")
	}

	_, _ = fmt.Fprintf(out, "### Request Schema\n\n")
	writeSchemaSection(out, version.Schema.OpenAPIV3Schema, 0, "")
	_, _ = fmt.Fprintf(out, "\n")
}

func buildOperations(kind, plural, listKind, scope, basePath string, hasStatus bool) []apiOperation {
	collectionParams := []string{"`pretty` (query, optional)", "`continue` (query, optional)", "`limit` (query, optional)", "`fieldSelector` (query, optional)", "`labelSelector` (query, optional)"}
	if scope == "Namespaced" {
		collectionParams = append([]string{"`namespace` (query)"}, collectionParams...)
	}
	nameParams := []string{"`name` (path)", "`pretty` (query, optional)"}
	if scope == "Namespaced" {
		nameParams = append([]string{"`namespace` (query)"}, nameParams...)
	}
	writeParams := []string{"`pretty` (query, optional)", "`dryRun` (query, optional)", "`fieldManager` (query, optional)", "`fieldValidation` (query, optional)"}
	if scope == "Namespaced" {
		writeParams = append([]string{"`namespace` (query)"}, writeParams...)
	}
	statusWriteParams := append([]string{}, nameParams...)
	statusWriteParams = append(statusWriteParams, "`fieldManager` (query, optional)", "`fieldValidation` (query, optional)")
	patchParams := append(append([]string{}, statusWriteParams...), "`force` (query, optional)")

	okCollection := []apiResponse{
		{200, "OK", listKind},
		{401, "Unauthorized", ""},
	}
	okSingle := []apiResponse{
		{200, "OK", kind},
		{401, "Unauthorized", ""},
		{404, "Not Found", ""},
	}
	created := []apiResponse{
		{201, "Created", kind},
		{202, "Accepted", kind},
		{401, "Unauthorized", ""},
	}
	accepted := []apiResponse{
		{200, "OK", kind},
		{202, "Accepted", kind},
		{401, "Unauthorized", ""},
		{404, "Not Found", ""},
	}
	deletedCollection := []apiResponse{
		{200, "OK", "Status"},
		{401, "Unauthorized", ""},
	}
	deletedSingle := []apiResponse{
		{200, "OK", "Status"},
		{202, "Accepted", "Status"},
		{401, "Unauthorized", ""},
		{404, "Not Found", ""},
	}

	ops := []apiOperation{
		{Method: "GET", Path: basePath, Description: fmt.Sprintf("List %s resources.", plural), Params: collectionParams, Responses: okCollection},
		{Method: "POST", Path: basePath, Description: fmt.Sprintf("Create a %s resource.", kind), Params: writeParams, Body: kind, Responses: created},
		{Method: "DELETE", Path: basePath, Description: fmt.Sprintf("Delete a collection of %s resources.", plural), Params: collectionParams, Responses: deletedCollection},
		{Method: "GET", Path: basePath + "/{name}", Description: fmt.Sprintf("Get a %s resource.", kind), Params: nameParams, Responses: okSingle},
		{Method: "PUT", Path: basePath + "/{name}", Description: fmt.Sprintf("Replace a %s resource.", kind), Params: append([]string{}, statusWriteParams...), Body: kind, Responses: okSingle},
		{Method: "PATCH", Path: basePath + "/{name}", Description: fmt.Sprintf("Patch a %s resource.", kind), Params: patchParams, Body: kind, Responses: okSingle},
		{Method: "DELETE", Path: basePath + "/{name}", Description: fmt.Sprintf("Delete a %s resource.", kind), Params: nameParams, Responses: deletedSingle},
	}
	if hasStatus {
		ops = append(ops,
			apiOperation{Method: "GET", Path: basePath + "/{name}/status", Description: fmt.Sprintf("Get the status subresource for %s.", kind), Params: nameParams, Responses: okSingle},
			apiOperation{Method: "PUT", Path: basePath + "/{name}/status", Description: fmt.Sprintf("Replace the status subresource for %s.", kind), Params: statusWriteParams, Body: kind, Responses: accepted},
			apiOperation{Method: "PATCH", Path: basePath + "/{name}/status", Description: fmt.Sprintf("Patch the status subresource for %s.", kind), Params: patchParams, Body: kind, Responses: accepted},
		)
	}
	return ops
}

func resourceBasePath(scope, group, version, plural string) string {
	return fmt.Sprintf("/api/v1/%s/%s/%s", group, version, plural)
}

func writeSchemaSection(out *bytes.Buffer, node schemaNode, depth int, name string) {
	if depth == 0 {
		for _, key := range []string{"apiVersion", "kind", "metadata", "spec", "status"} {
			child, ok := node.Properties[key]
			if !ok {
				continue
			}
			writeSchemaNode(out, key, child, 0, requiredSet(node.Required))
		}
		return
	}
	writeSchemaNode(out, name, node, depth, nil)
}

func writeSchemaNode(out *bytes.Buffer, name string, node schemaNode, depth int, required map[string]bool) {
	indent := strings.Repeat("  ", depth)
	typeName := schemaType(node)
	req := "optional"
	if required != nil && required[name] {
		req = "required"
	}
	line := fmt.Sprintf("%s- `%s`: `%s`, %s", indent, name, typeName, req)
	if len(node.Enum) > 0 {
		line += fmt.Sprintf(", enum=%s", enumValues(node.Enum))
	}
	if node.Description != "" {
		line += fmt.Sprintf(" - %s", trimDescription(node.Description))
	}
	_, _ = fmt.Fprintf(out, "%s\n", line)

	if shouldCollapseNode(name, node, depth) {
		return
	}

	childRequired := requiredSet(node.Required)
	keys := sortedKeys(node.Properties)
	for _, key := range keys {
		writeSchemaNode(out, key, node.Properties[key], depth+1, childRequired)
	}
	if node.Items != nil && len(node.Items.Properties) > 0 {
		writeSchemaNode(out, "items", *node.Items, depth+1, requiredSet(node.Items.Required))
	}
	if node.AdditionalProperties != nil && len(node.AdditionalProperties.Properties) > 0 {
		writeSchemaNode(out, "additionalProperties", *node.AdditionalProperties, depth+1, requiredSet(node.AdditionalProperties.Required))
	}
}

func schemaType(node schemaNode) string {
	if node.XIntOrString {
		return "integer|string"
	}
	if node.Type == "array" && node.Items != nil {
		return "array"
	}
	if node.Type != "" {
		return node.Type
	}
	if node.AdditionalProperties != nil {
		return "object"
	}
	return "object"
}

func requiredSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]bool, len(items))
	for _, item := range items {
		result[item] = true
	}
	return result
}

func shouldCollapseNode(name string, node schemaNode, depth int) bool {
	if depth >= 3 && len(node.Properties) > 0 {
		return true
	}
	if name == "template" && len(node.Properties) > 0 {
		return true
	}
	if name == "metadata" && depth >= 1 {
		return true
	}
	return false
}

func sortedKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j])
	})
	return keys
}

func trimDescription(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > 120 {
		return string(runes[:117]) + "..."
	}
	return s
}

func enumValues(items []any) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprint(item))
	}
	return strings.Join(parts, ",")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
