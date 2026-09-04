package envelopes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

// SchemaDocument is the module-owned source representation of a compiled
// JSON Schema. It keeps the original JSON and a parsed document together so
// consumers do not need to find or re-parse files from the module checkout.
//
// SchemaDocument values returned by this package are immutable. JSON returns
// a copy of the source bytes, and metadata accessors return copied values.
type SchemaDocument struct {
	uri      string
	raw      json.RawMessage
	document any
}

// NewSchemaDocument parses a JSON Schema source document and associates it
// with the stable URI used when compiling it.
func NewSchemaDocument(uri string, data []byte) (*SchemaDocument, error) {
	if uri == "" {
		return nil, fmt.Errorf("envelopes: schema URI is empty")
	}
	var document any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&document); err != nil {
		return nil, fmt.Errorf("envelopes: parse schema %s: %w", uri, err)
	}
	switch document.(type) {
	case map[string]any, bool:
		// JSON Schema permits object and boolean documents.
	default:
		return nil, fmt.Errorf("envelopes: schema %s is neither an object nor a boolean", uri)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("envelopes: schema %s has trailing JSON value", uri)
		}
		return nil, fmt.Errorf("envelopes: parse schema %s trailing content: %w", uri, err)
	}
	compact := new(bytes.Buffer)
	if err := json.Compact(compact, data); err != nil {
		return nil, fmt.Errorf("envelopes: compact schema %s: %w", uri, err)
	}
	return &SchemaDocument{
		uri:      uri,
		raw:      append(json.RawMessage(nil), compact.Bytes()...),
		document: document,
	}, nil
}

// URI returns the stable resource URI used to compile the schema.
func (d *SchemaDocument) URI() string {
	if d == nil {
		return ""
	}
	return d.uri
}

// JSON returns a copy of the original JSON Schema document.
func (d *SchemaDocument) JSON() json.RawMessage {
	if d == nil {
		return nil
	}
	return append(json.RawMessage(nil), d.raw...)
}

// Metadata returns metadata for the schema root.
func (d *SchemaDocument) Metadata() SchemaMetadata {
	metadata, _ := d.MetadataAtSchemaPath("")
	return metadata
}

// MetadataAtInstancePath returns the schema metadata that applies at an RFC
// 6901 instance pointer. It follows object properties, array items, and local
// $ref values. For schemas whose applicator logic cannot identify one unique
// node (for example a oneOf with several alternatives), ok is false.
func (d *SchemaDocument) MetadataAtInstancePath(pointer string) (metadata SchemaMetadata, ok bool) {
	if d == nil {
		return SchemaMetadata{}, false
	}
	parts, err := parseJSONPointer(pointer)
	if err != nil {
		return SchemaMetadata{}, false
	}
	node, rootOK := d.document.(map[string]any)
	if !rootOK {
		return SchemaMetadata{}, false
	}
	path := []string{}
	for _, part := range parts {
		resolved, resolvedPath, found := d.resolveLocalRef(node, path, map[string]bool{})
		if !found {
			return SchemaMetadata{}, false
		}
		node, path = resolved, resolvedPath
		if properties, _ := node["properties"].(map[string]any); properties != nil {
			child, _ := properties[part].(map[string]any)
			if child == nil {
				return SchemaMetadata{}, false
			}
			node = child
			path = append(path, "properties", part)
			continue
		}
		items, _ := node["items"].(map[string]any)
		if items == nil {
			return SchemaMetadata{}, false
		}
		node = items
		path = append(path, "items")
	}
	node, path, ok = d.resolveLocalRef(node, path, map[string]bool{})
	if !ok {
		return SchemaMetadata{}, false
	}
	return metadataFromNode(jsonPointer(path), node), true
}

// MetadataAtSchemaPath returns metadata at an RFC 6901 pointer within the
// schema document. A trailing validation keyword (such as /required or /type)
// may be present; the metadata for the containing schema node is returned.
func (d *SchemaDocument) MetadataAtSchemaPath(pointer string) (SchemaMetadata, bool) {
	if d == nil {
		return SchemaMetadata{}, false
	}
	parts, err := parseJSONPointer(pointer)
	if err != nil {
		return SchemaMetadata{}, false
	}
	root, rootOK := d.document.(map[string]any)
	if !rootOK {
		return SchemaMetadata{}, false
	}
	node := any(root)
	consumed := make([]string, 0, len(parts))
	lastObject, lastPath := root, []string{}
	for _, part := range parts {
		object, objectOK := node.(map[string]any)
		if !objectOK {
			return SchemaMetadata{}, false
		}
		lastObject = object
		lastPath = append([]string(nil), consumed...)
		next, found := object[part]
		if !found {
			return SchemaMetadata{}, false
		}
		node = next
		consumed = append(consumed, part)
	}
	if object, objectOK := node.(map[string]any); objectOK {
		lastObject = object
		lastPath = consumed
	}
	resolved, resolvedPath, ok := d.resolveLocalRef(lastObject, lastPath, map[string]bool{})
	if !ok {
		return SchemaMetadata{}, false
	}
	return metadataFromNode(jsonPointer(resolvedPath), resolved), true
}

func (d *SchemaDocument) resolveLocalRef(node map[string]any, path []string, seen map[string]bool) (map[string]any, []string, bool) {
	ref, _ := node["$ref"].(string)
	if ref == "" {
		return node, path, true
	}
	if !strings.HasPrefix(ref, "#") {
		return nil, nil, false
	}
	if seen[ref] {
		return nil, nil, false
	}
	seen[ref] = true
	parts, err := parseJSONPointer(strings.TrimPrefix(ref, "#"))
	if err != nil {
		return nil, nil, false
	}
	value := d.document
	for _, part := range parts {
		object, objectOK := value.(map[string]any)
		if !objectOK {
			return nil, nil, false
		}
		value, objectOK = object[part]
		if !objectOK {
			return nil, nil, false
		}
	}
	resolved, ok := value.(map[string]any)
	if !ok {
		return nil, nil, false
	}
	return d.resolveLocalRef(resolved, parts, seen)
}

// SchemaMetadata exposes the standard annotations and validation vocabulary
// most useful for diagnostics and generators. Custom contains only unknown
// extension keywords, such as default_render_target; generic library code
// preserves them but assigns them no host-specific meaning.
type SchemaMetadata struct {
	SchemaPath                  string         `json:"schemaPath"`
	Title                       string         `json:"title,omitempty"`
	Description                 string         `json:"description,omitempty"`
	Default                     any            `json:"default,omitempty"`
	HasDefault                  bool           `json:"hasDefault,omitempty"`
	Examples                    []any          `json:"examples,omitempty"`
	ReadOnly                    bool           `json:"readOnly,omitempty"`
	WriteOnly                   bool           `json:"writeOnly,omitempty"`
	Deprecated                  bool           `json:"deprecated,omitempty"`
	Types                       []string       `json:"types,omitempty"`
	Enum                        []any          `json:"enum,omitempty"`
	Required                    []string       `json:"required,omitempty"`
	Properties                  []string       `json:"properties,omitempty"`
	AdditionalPropertiesAllowed *bool          `json:"additionalPropertiesAllowed,omitempty"`
	Custom                      map[string]any `json:"custom,omitempty"`
}

func metadataFromNode(path string, node map[string]any) SchemaMetadata {
	metadata := SchemaMetadata{SchemaPath: path}
	metadata.Title, _ = node["title"].(string)
	metadata.Description, _ = node["description"].(string)
	if value, exists := node["default"]; exists {
		metadata.HasDefault = true
		metadata.Default = cloneJSONValue(value)
	}
	metadata.Examples = anySlice(node["examples"])
	metadata.ReadOnly, _ = node["readOnly"].(bool)
	metadata.WriteOnly, _ = node["writeOnly"].(bool)
	metadata.Deprecated, _ = node["deprecated"].(bool)
	metadata.Types = schemaTypes(node["type"])
	metadata.Enum = anySlice(node["enum"])
	metadata.Required = stringSlice(node["required"])
	if properties, _ := node["properties"].(map[string]any); len(properties) > 0 {
		metadata.Properties = make([]string, 0, len(properties))
		for name := range properties {
			metadata.Properties = append(metadata.Properties, name)
		}
		sort.Strings(metadata.Properties)
	}
	if allowed, ok := node["additionalProperties"].(bool); ok {
		metadata.AdditionalPropertiesAllowed = new(bool)
		*metadata.AdditionalPropertiesAllowed = allowed
	}
	for name, value := range node {
		if knownSchemaKeyword(name) {
			continue
		}
		if metadata.Custom == nil {
			metadata.Custom = make(map[string]any)
		}
		metadata.Custom[name] = cloneJSONValue(value)
	}
	return metadata
}

func schemaTypes(value any) []string {
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		return []string{s}
	}
	return stringSlice(value)
}

func stringSlice(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func anySlice(value any) []any {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]any, len(values))
	for i := range values {
		out[i] = cloneJSONValue(values[i])
	}
	return out
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, child := range value {
			out[key] = cloneJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i := range value {
			out[i] = cloneJSONValue(value[i])
		}
		return out
	default:
		return value
	}
}

// normalizeJSONValue converts any value accepted by encoding/json into the
// package's canonical JSON-shaped representation. In particular, typed maps,
// typed slices, structs, arrays, and pointers become independently owned
// map[string]any / []any / scalar values. Registry storage uses this at the
// ownership boundary so later caller mutation cannot race with reads.
func normalizeJSONValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeStringAnyMap(values map[string]any) (map[string]any, error) {
	if values == nil {
		return nil, nil
	}
	normalized, err := normalizeJSONValue(values)
	if err != nil {
		return nil, err
	}
	result, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("normalized metadata is not an object")
	}
	return result, nil
}

func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("invalid JSON pointer %q", pointer)
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i, part := range parts {
		part = strings.ReplaceAll(part, "~1", "/")
		part = strings.ReplaceAll(part, "~0", "~")
		parts[i] = part
	}
	return parts, nil
}

func jsonPointer(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		b.WriteByte('/')
		part = strings.ReplaceAll(part, "~", "~0")
		part = strings.ReplaceAll(part, "/", "~1")
		b.WriteString(part)
	}
	return b.String()
}

func schemaPathFromLocation(location string, keywordPath []string) string {
	fragment := ""
	if parsed, err := url.Parse(location); err == nil {
		fragment, _ = url.PathUnescape(parsed.Fragment)
	} else if index := strings.IndexByte(location, '#'); index >= 0 {
		fragment = location[index+1:]
	}
	if fragment != "" && !strings.HasPrefix(fragment, "/") {
		fragment = ""
	}
	parts, err := parseJSONPointer(fragment)
	if err != nil {
		parts = nil
	}
	parts = append(parts, keywordPath...)
	return jsonPointer(parts)
}

func knownSchemaKeyword(name string) bool {
	_, ok := standardSchemaKeywords[name]
	return ok
}

var standardSchemaKeywords = map[string]struct{}{
	"$anchor": {}, "$comment": {}, "$defs": {}, "$dynamicAnchor": {}, "$dynamicRef": {},
	"$id": {}, "$ref": {}, "$schema": {}, "$vocabulary": {},
	"additionalItems": {}, "additionalProperties": {}, "allOf": {}, "anyOf": {},
	"const": {}, "contains": {}, "contentEncoding": {}, "contentMediaType": {},
	"contentSchema": {}, "default": {}, "dependentRequired": {}, "dependentSchemas": {},
	"deprecated": {}, "description": {}, "else": {}, "enum": {}, "examples": {},
	"exclusiveMaximum": {}, "exclusiveMinimum": {}, "format": {}, "if": {}, "items": {},
	"maxContains": {}, "maximum": {}, "maxItems": {}, "maxLength": {}, "maxProperties": {},
	"minContains": {}, "minimum": {}, "minItems": {}, "minLength": {}, "minProperties": {},
	"multipleOf": {}, "not": {}, "oneOf": {}, "pattern": {}, "patternProperties": {},
	"prefixItems": {}, "properties": {}, "propertyNames": {}, "readOnly": {}, "required": {},
	"then": {}, "title": {}, "type": {}, "unevaluatedItems": {},
	"unevaluatedProperties": {}, "uniqueItems": {}, "writeOnly": {},
}
