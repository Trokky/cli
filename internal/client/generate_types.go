package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Schema is a Trokky collection schema as returned by GET /schemas/<name>.
// Fields is an ordered slice: declaration order from the server is preserved,
// whether the wire format uses the record form or the array form.
type Schema struct {
	Name        string
	Title       string
	Type        string // "document" or "singleton"
	Description string
	Fields      []Field
}

// Field is a single field definition inside a schema.
type Field struct {
	Name        string
	Type        string
	Title       string
	Description string
	Required    bool
	Fields      []Field // nested fields for object types
	Of          *Field  // item definition for array types (legacy: "items")
	To          []string
}

// --- JSON decoding (order preserving) ---

type rawPair struct {
	Key   string
	Value json.RawMessage
}

// decodeOrderedObject walks a JSON object with a token decoder so that key
// order is preserved (Go maps do not preserve it).
func decodeOrderedObject(data []byte) ([]rawPair, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}

	var pairs []rawPair
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected object key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		pairs = append(pairs, rawPair{Key: key, Value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return pairs, nil
}

func jsonKind(data []byte) byte {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}

// UnmarshalJSON parses a schema object, preserving field declaration order.
func (s *Schema) UnmarshalJSON(data []byte) error {
	var aux struct {
		Name        string          `json:"name"`
		Title       string          `json:"title"`
		Type        string          `json:"type"`
		Description string          `json:"description"`
		Fields      json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	s.Name = aux.Name
	s.Title = aux.Title
	s.Type = aux.Type
	s.Description = aux.Description

	fields, err := parseFields(aux.Fields)
	if err != nil {
		return err
	}
	s.Fields = fields
	return nil
}

// UnmarshalJSON parses a field definition, tolerating both the record and the
// array form for nested fields and both "of" and the legacy "items" for arrays.
func (f *Field) UnmarshalJSON(data []byte) error {
	var aux struct {
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Title       string          `json:"title"`
		Description string          `json:"description"`
		Required    bool            `json:"required"`
		Fields      json.RawMessage `json:"fields"`
		Of          json.RawMessage `json:"of"`
		Items       json.RawMessage `json:"items"`
		To          json.RawMessage `json:"to"`
		Options     *struct {
			To     json.RawMessage `json:"to"`
			Fields json.RawMessage `json:"fields"`
		} `json:"options"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	f.Name = aux.Name
	f.Type = aux.Type
	f.Title = aux.Title
	f.Description = aux.Description
	f.Required = aux.Required

	nested := aux.Fields
	if len(nested) == 0 && aux.Options != nil {
		nested = aux.Options.Fields
	}
	fields, err := parseFields(nested)
	if err != nil {
		return err
	}
	f.Fields = fields

	item := aux.Of
	if len(item) == 0 {
		item = aux.Items
	}
	if len(item) > 0 && jsonKind(item) == '{' {
		var of Field
		if err := json.Unmarshal(item, &of); err != nil {
			return err
		}
		f.Of = &of
	}

	to := aux.To
	if len(to) == 0 && aux.Options != nil {
		to = aux.Options.To
	}
	f.To = parseTo(to)
	return nil
}

func parseTo(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	switch jsonKind(data) {
	case '"':
		var s string
		if json.Unmarshal(data, &s) == nil && s != "" {
			return []string{s}
		}
	case '[':
		// Either ["a","b"] or [{"type":"a"}]
		var list []string
		if json.Unmarshal(data, &list) == nil {
			return list
		}
		var objs []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &objs) == nil {
			var out []string
			for _, o := range objs {
				if o.Type != "" {
					out = append(out, o.Type)
				} else if o.Name != "" {
					out = append(out, o.Name)
				}
			}
			return out
		}
	}
	return nil
}

// parseFields accepts the record form ({"title": {...}}) or the array form
// ([{"name":"title", ...}]) and returns fields in declaration order.
func parseFields(data []byte) ([]Field, error) {
	if len(data) == 0 || jsonKind(data) == 'n' {
		return nil, nil
	}

	switch jsonKind(data) {
	case '[':
		var fields []Field
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, err
		}
		return fields, nil
	case '{':
		pairs, err := decodeOrderedObject(data)
		if err != nil {
			return nil, err
		}
		fields := make([]Field, 0, len(pairs))
		for _, p := range pairs {
			if jsonKind(p.Value) != '{' {
				continue
			}
			var f Field
			if err := json.Unmarshal(p.Value, &f); err != nil {
				return nil, err
			}
			f.Name = p.Key
			fields = append(fields, f)
		}
		return fields, nil
	}
	return nil, fmt.Errorf("unexpected fields format")
}

// parseCollectionNames handles {"collections":[...]}, a bare array of schema
// objects, and a bare array of names.
func parseCollectionNames(data []byte) ([]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty response from /collections")
	}

	payload := data
	if jsonKind(payload) == '{' {
		var wrapper struct {
			Collections json.RawMessage `json:"collections"`
		}
		if err := json.Unmarshal(payload, &wrapper); err != nil {
			return nil, err
		}
		if len(wrapper.Collections) == 0 {
			return nil, fmt.Errorf("unexpected /collections response format")
		}
		payload = wrapper.Collections
	}

	if jsonKind(payload) != '[' {
		return nil, fmt.Errorf("unexpected /collections response format")
	}

	var names []string
	if json.Unmarshal(payload, &names) == nil {
		return names, nil
	}

	var objs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &objs); err != nil {
		return nil, fmt.Errorf("unexpected /collections response format")
	}
	names = make([]string, 0, len(objs))
	for _, o := range objs {
		if o.Name != "" {
			names = append(names, o.Name)
		}
	}
	return names, nil
}

// parseSchema handles {"schema":{...}} and a bare schema object.
func parseSchema(data []byte) (Schema, error) {
	var schema Schema
	if jsonKind(data) != '{' {
		return schema, fmt.Errorf("unexpected schema response format")
	}

	var wrapper struct {
		Schema json.RawMessage `json:"schema"`
	}
	if json.Unmarshal(data, &wrapper) == nil && len(wrapper.Schema) > 0 && jsonKind(wrapper.Schema) == '{' {
		data = wrapper.Schema
	}

	if err := json.Unmarshal(data, &schema); err != nil {
		return Schema{}, err
	}
	return schema, nil
}

// GenerateTypes fetches every collection schema from the server and renders a
// single self-contained TypeScript definition file.
func (c *Client) GenerateTypes() (string, error) {
	data, err := c.Get("/collections")
	if err != nil {
		return "", err
	}

	names, err := parseCollectionNames(data)
	if err != nil {
		return "", err
	}

	schemas := make([]Schema, 0, len(names))
	for _, name := range names {
		raw, err := c.Get("/schemas/" + url.PathEscape(name))
		if err != nil {
			return "", fmt.Errorf("failed to fetch schema for %q: %w", name, err)
		}
		schema, err := parseSchema(raw)
		if err != nil {
			return "", fmt.Errorf("failed to parse schema for %q: %w", name, err)
		}
		if schema.Name == "" {
			schema.Name = name
		}
		schemas = append(schemas, schema)
	}

	return generateTypeScript(schemas, c.BaseURL), nil
}

// GenerateTypeScript renders schemas into one self-contained TypeScript file.
// It performs no I/O and imports nothing, so the output compiles standalone.
func GenerateTypeScript(schemas []Schema) string {
	return generateTypeScript(schemas, "")
}

func generateTypeScript(schemas []Schema, source string) string {
	var b strings.Builder

	b.WriteString("// Generated by trokky CLI\n")
	if source != "" {
		fmt.Fprintf(&b, "// Source: %s\n", source)
	}
	b.WriteString("// Do not edit manually.\n\n")

	b.WriteString("export interface BaseDocument {\n")
	b.WriteString("  _id: string\n")
	b.WriteString("  _type: string\n")
	b.WriteString("  _createdAt?: string\n")
	b.WriteString("  _updatedAt?: string\n")
	b.WriteString("  _version?: number\n")
	b.WriteString("  _status?: 'draft' | 'published'\n")
	b.WriteString("}\n\n")

	b.WriteString("export interface MediaAssetReference {\n")
	b.WriteString("  _ref: string\n")
	b.WriteString("  _type: 'mediaAsset'\n")
	b.WriteString("}\n\n")

	b.WriteString("export interface MediaFieldValue {\n")
	b.WriteString("  _type: 'media'\n")
	b.WriteString("  asset: MediaAssetReference\n")
	b.WriteString("  alt?: string\n")
	b.WriteString("  caption?: string\n")
	b.WriteString("  title?: string\n")
	b.WriteString("  variant?: string\n")
	b.WriteString("}\n\n")

	// Rich text is stored as a ProseMirror document ({ type: 'doc', content: [...] }),
	// see studio RichTextField/format-converter.ts. Portable text is an array of blocks.
	b.WriteString("export interface RichTextValue {\n")
	b.WriteString("  type: 'doc'\n")
	b.WriteString("  content?: unknown[]\n")
	b.WriteString("}\n\n")

	b.WriteString("export interface Reference<T extends string = string> {\n")
	b.WriteString("  _ref: string\n")
	b.WriteString("  _type: T\n")
	b.WriteString("}\n")

	known := make(map[string]bool, len(schemas))
	for _, s := range schemas {
		if s.Name != "" {
			known[s.Name] = true
		}
	}

	for _, schema := range schemas {
		b.WriteString("\n")
		b.WriteString(renderSchema(schema, known))
	}

	b.WriteString("\n")
	b.WriteString(renderIndex(schemas))

	return b.String()
}

func renderSchema(schema Schema, known map[string]bool) string {
	docName := pascalCase(schema.Name)
	var b strings.Builder

	// Nested interfaces first — TS hoists, but this reads like the reference
	// generator's output.
	for _, f := range schema.Fields {
		b.WriteString(renderNestedInterfaces(f, docName, known))
	}

	b.WriteString("/**\n")
	title := schema.Title
	if title == "" {
		title = schema.Name
	}
	fmt.Fprintf(&b, " * %s\n", title)
	if schema.Description != "" {
		fmt.Fprintf(&b, " * %s\n", schema.Description)
	}
	if schema.Type != "" {
		fmt.Fprintf(&b, " * Type: %s\n", schema.Type)
	}
	b.WriteString(" */\n")

	fmt.Fprintf(&b, "export interface %sDocument extends BaseDocument {\n", docName)
	fmt.Fprintf(&b, "  _type: '%s'\n", schema.Name)
	for _, f := range schema.Fields {
		b.WriteString(renderFieldLine(f, docName, known))
	}
	b.WriteString("}\n")

	return b.String()
}

// renderNestedInterfaces emits interfaces for object fields and for arrays of
// objects, innermost first.
func renderNestedInterfaces(f Field, prefix string, known map[string]bool) string {
	var b strings.Builder

	switch strings.ToLower(f.Type) {
	case "object":
		name := prefix + pascalCase(f.Name)
		for _, sub := range f.Fields {
			b.WriteString(renderNestedInterfaces(sub, name, known))
		}
		fmt.Fprintf(&b, "export interface %s {\n", name)
		for _, sub := range f.Fields {
			b.WriteString(renderFieldLine(sub, name, known))
		}
		b.WriteString("}\n\n")
	case "array":
		if f.Of == nil {
			return ""
		}
		if strings.ToLower(f.Of.Type) == "object" {
			name := prefix + pascalCase(f.Name) + "Item"
			for _, sub := range f.Of.Fields {
				b.WriteString(renderNestedInterfaces(sub, name, known))
			}
			fmt.Fprintf(&b, "export interface %s {\n", name)
			for _, sub := range f.Of.Fields {
				b.WriteString(renderFieldLine(sub, name, known))
			}
			b.WriteString("}\n\n")
		}
	}

	return b.String()
}

func renderFieldLine(f Field, prefix string, known map[string]bool) string {
	optional := "?"
	if f.Required {
		optional = ""
	}
	return fmt.Sprintf("  %s%s: %s\n", propertyName(f.Name), optional, mapFieldType(f, prefix, known))
}

func mapFieldType(f Field, prefix string, known map[string]bool) string {
	switch strings.ToLower(f.Type) {
	case "string", "text", "slug", "email", "url", "password", "color":
		return "string"
	case "number", "integer", "float":
		return "number"
	case "boolean":
		return "boolean"
	case "date", "datetime":
		return "string"
	case "richtext":
		return "RichTextValue"
	case "portable", "portabletext", "blockcontent":
		return "unknown[]"
	case "json":
		return "unknown"
	case "media", "image", "audio", "video", "document", "file":
		return "MediaFieldValue | null"
	case "reference":
		return referenceType(f.To, known)
	case "array":
		if f.Of == nil {
			return "unknown[]"
		}
		item := f.Of
		var itemType string
		if strings.ToLower(item.Type) == "object" {
			itemType = prefix + pascalCase(f.Name) + "Item"
		} else {
			// The item definition borrows the array's name so nested naming
			// (references, unions) stays stable.
			named := *item
			named.Name = f.Name
			itemType = mapFieldType(named, prefix, known)
		}
		if strings.Contains(itemType, " | ") {
			return "(" + itemType + ")[]"
		}
		return itemType + "[]"
	case "object":
		return prefix + pascalCase(f.Name)
	default:
		return "unknown"
	}
}

func referenceType(to []string, known map[string]bool) string {
	targets := make([]string, 0, len(to))
	for _, t := range to {
		if t != "" {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 {
		return "Reference"
	}

	quoted := make([]string, len(targets))
	for i, t := range targets {
		quoted[i] = "'" + t + "'"
	}
	parts := []string{"Reference<" + strings.Join(quoted, " | ") + ">"}
	for _, t := range targets {
		if known[t] {
			parts = append(parts, pascalCase(t)+"Document")
		}
	}
	return strings.Join(parts, " | ")
}

func renderIndex(schemas []Schema) string {
	var b strings.Builder

	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if s.Name != "" {
			names = append(names, s.Name)
		}
	}

	if len(names) == 0 {
		b.WriteString("export type DocumentType = never\n\n")
		b.WriteString("export type AnyDocument = never\n\n")
		b.WriteString("export interface DocumentTypeMap {\n}\n\n")
		b.WriteString("export type DocumentOf<T extends DocumentType> = DocumentTypeMap[T]\n")
		return b.String()
	}

	quoted := make([]string, len(names))
	docs := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
		docs[i] = pascalCase(n) + "Document"
	}

	fmt.Fprintf(&b, "export type DocumentType = %s\n\n", strings.Join(quoted, " | "))
	fmt.Fprintf(&b, "export type AnyDocument = %s\n\n", strings.Join(docs, " | "))

	b.WriteString("export interface DocumentTypeMap {\n")
	for i, n := range names {
		fmt.Fprintf(&b, "  '%s': %s\n", n, docs[i])
	}
	b.WriteString("}\n\n")
	b.WriteString("export type DocumentOf<T extends DocumentType> = DocumentTypeMap[T]\n")

	return b.String()
}

// --- identifier helpers ---

// pascalCase converts kebab-case, snake_case and camelCase names into a valid
// PascalCase TypeScript identifier.
func pascalCase(s string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			if upperNext && r >= 'a' && r <= 'z' {
				b.WriteRune(r - 32)
			} else {
				b.WriteRune(r)
			}
			upperNext = false
		default:
			upperNext = true
		}
	}

	out := b.String()
	if out == "" {
		return "Unnamed"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "_" + out
	}
	return out
}

// propertyName quotes field names that are not valid TypeScript identifiers.
func propertyName(name string) string {
	if isIdentifier(name) {
		return name
	}
	return "'" + strings.ReplaceAll(name, "'", "\\'") + "'"
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '$'
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isAlpha {
			return false
		}
		if i > 0 && !isAlpha && !isDigit {
			return false
		}
	}
	return !reservedWords[s]
}

var reservedWords = func() map[string]bool {
	words := []string{
		"break", "case", "catch", "class", "const", "continue", "debugger",
		"default", "delete", "do", "else", "enum", "export", "extends", "false",
		"finally", "for", "function", "if", "import", "in", "instanceof", "new",
		"null", "return", "super", "switch", "this", "throw", "true", "try",
		"typeof", "var", "void", "while", "with",
	}
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}()
