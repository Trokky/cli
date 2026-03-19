package backup

import (
	"testing"
)

func TestValidateDocument_AllValid(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string", Required: true},
			{Name: "count", Type: "number"},
			{Name: "active", Type: "boolean"},
		},
	}
	doc := map[string]interface{}{
		"title":  "Hello",
		"count":  float64(42),
		"active": true,
	}

	result := ValidateDocument(doc, schema)
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateDocument_MissingRequired(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string", Required: true},
			{Name: "body", Type: "string", Required: true},
		},
	}
	doc := map[string]interface{}{
		"title": "Hello",
	}

	result := ValidateDocument(doc, schema)
	if result.Valid {
		t.Fatal("expected invalid")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Field != "body" {
		t.Fatalf("expected error on 'body', got %q", result.Errors[0].Field)
	}
}

func TestValidateDocument_NullRequired(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string", Required: true},
		},
	}
	doc := map[string]interface{}{
		"title": nil,
	}

	result := ValidateDocument(doc, schema)
	if result.Valid {
		t.Fatal("expected invalid for nil required field")
	}
}

func TestValidateDocument_WrongType(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string"},
			{Name: "count", Type: "number"},
		},
	}
	doc := map[string]interface{}{
		"title": 123,       // should be string
		"count": "not-num", // should be number
	}

	result := ValidateDocument(doc, schema)
	if result.Valid {
		t.Fatal("expected invalid")
	}
	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

func TestValidateDocument_OptionalMissing(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string", Required: true},
			{Name: "description", Type: "string"},
		},
	}
	doc := map[string]interface{}{
		"title": "Hello",
		// description is optional and missing — should be fine
	}

	result := ValidateDocument(doc, schema)
	if !result.Valid {
		t.Fatalf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateDocument_ArrayType(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "tags", Type: "array"},
		},
	}

	valid := map[string]interface{}{"tags": []interface{}{"a", "b"}}
	invalid := map[string]interface{}{"tags": "not-array"}

	if r := ValidateDocument(valid, schema); !r.Valid {
		t.Fatal("expected valid for array")
	}
	if r := ValidateDocument(invalid, schema); r.Valid {
		t.Fatal("expected invalid for non-array")
	}
}

func TestValidateDocument_ObjectType(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "meta", Type: "object"},
		},
	}

	valid := map[string]interface{}{"meta": map[string]interface{}{"k": "v"}}
	invalid := map[string]interface{}{"meta": "not-object"}

	if r := ValidateDocument(valid, schema); !r.Valid {
		t.Fatal("expected valid for object")
	}
	if r := ValidateDocument(invalid, schema); r.Valid {
		t.Fatal("expected invalid for non-object")
	}
}

func TestValidateDocument_EmptySchema(t *testing.T) {
	schema := SchemaDefinition{}
	doc := map[string]interface{}{"anything": "goes"}

	result := ValidateDocument(doc, schema)
	if !result.Valid {
		t.Fatal("empty schema should accept anything")
	}
}

func TestValidateDocument_ExtraFields(t *testing.T) {
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string"},
		},
	}
	doc := map[string]interface{}{
		"title":       "Hello",
		"extra_field": "should be ignored",
	}

	result := ValidateDocument(doc, schema)
	if !result.Valid {
		t.Fatal("extra fields should not cause validation failure")
	}
}

func TestValidationError_String(t *testing.T) {
	e := ValidationError{Field: "title", Message: "required field is missing"}
	if e.String() != "title: required field is missing" {
		t.Fatalf("got %q", e.String())
	}
}
