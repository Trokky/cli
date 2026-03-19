package backup

import (
	"encoding/json"
	"testing"
)

// --- UpdateReferences ---

func TestUpdateReferences_StringRef(t *testing.T) {
	doc := map[string]interface{}{
		"title":  "Post 1",
		"author": "old-author-id",
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "title", Type: "string"},
			{Name: "author", Type: "reference", To: json.RawMessage(`"authors"`)},
		},
	}
	mappings := map[string]string{"old-author-id": "new-author-id"}

	updated, count := UpdateReferences(doc, schema, mappings)
	if count != 1 {
		t.Fatalf("expected 1 ref updated, got %d", count)
	}
	if updated["author"] != "new-author-id" {
		t.Fatalf("author = %v", updated["author"])
	}
}

func TestUpdateReferences_ObjectRef(t *testing.T) {
	doc := map[string]interface{}{
		"author": map[string]interface{}{"_ref": "old-id"},
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "author", Type: "reference", To: json.RawMessage(`"authors"`)},
		},
	}
	mappings := map[string]string{"old-id": "new-id"}

	updated, count := UpdateReferences(doc, schema, mappings)
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
	ref := updated["author"].(map[string]interface{})["_ref"]
	if ref != "new-id" {
		t.Fatalf("_ref = %v", ref)
	}
}

func TestUpdateReferences_NoMapping(t *testing.T) {
	doc := map[string]interface{}{
		"author": "unknown-id",
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "author", Type: "reference", To: json.RawMessage(`"authors"`)},
		},
	}

	_, count := UpdateReferences(doc, schema, map[string]string{})
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestUpdateReferences_ArrayOfRefs(t *testing.T) {
	doc := map[string]interface{}{
		"tags": []interface{}{
			map[string]interface{}{"_ref": "tag-1"},
			map[string]interface{}{"_ref": "tag-2"},
		},
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "tags", Type: "array", Of: &FieldDefinition{
				Name: "tag", Type: "reference", To: json.RawMessage(`"tags"`),
			}},
		},
	}
	mappings := map[string]string{"tag-1": "new-tag-1", "tag-2": "new-tag-2"}

	_, count := UpdateReferences(doc, schema, mappings)
	if count != 2 {
		t.Fatalf("expected 2, got %d", count)
	}
}

func TestUpdateReferences_NestedObject(t *testing.T) {
	doc := map[string]interface{}{
		"meta": map[string]interface{}{
			"category": "old-cat",
		},
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "meta", Type: "object", Fields: []FieldDefinition{
				{Name: "category", Type: "reference", To: json.RawMessage(`"categories"`)},
			}},
		},
	}
	mappings := map[string]string{"old-cat": "new-cat"}

	_, count := UpdateReferences(doc, schema, mappings)
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
}

func TestUpdateReferences_MediaAsset(t *testing.T) {
	doc := map[string]interface{}{
		"cover": map[string]interface{}{
			"asset": map[string]interface{}{"_ref": "media-old"},
		},
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "cover", Type: "image"},
		},
	}
	mappings := map[string]string{"media-old": "media-new"}

	_, count := UpdateReferences(doc, schema, mappings)
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
	asset := doc["cover"].(map[string]interface{})["asset"].(map[string]interface{})
	if asset["_ref"] != "media-new" {
		t.Fatalf("_ref = %v", asset["_ref"])
	}
}

func TestUpdateReferences_NilField(t *testing.T) {
	doc := map[string]interface{}{
		"author": nil,
	}
	schema := SchemaDefinition{
		Fields: []FieldDefinition{
			{Name: "author", Type: "reference", To: json.RawMessage(`"authors"`)},
		},
	}

	_, count := UpdateReferences(doc, schema, map[string]string{"x": "y"})
	if count != 0 {
		t.Fatalf("expected 0 for nil field, got %d", count)
	}
}

// --- SanitizeDocument ---

func TestSanitizeDocument_RemovesNulls(t *testing.T) {
	doc := map[string]interface{}{
		"title": "Hello",
		"body":  nil,
	}
	result := SanitizeDocument(doc)
	if _, ok := result["body"]; ok {
		t.Fatal("null field should be removed")
	}
	if result["title"] != "Hello" {
		t.Fatal("non-null field should be preserved")
	}
}

func TestSanitizeDocument_RemovesEmptyObjects(t *testing.T) {
	doc := map[string]interface{}{
		"title": "Hello",
		"meta":  map[string]interface{}{},
	}
	result := SanitizeDocument(doc)
	if _, ok := result["meta"]; ok {
		t.Fatal("empty object should be removed")
	}
}

func TestSanitizeDocument_RemovesOldMediaFormat(t *testing.T) {
	doc := map[string]interface{}{
		"cover": map[string]interface{}{
			"_type": "media",
			"src":   "/old/path.jpg",
			"alt":   "photo",
		},
	}
	result := SanitizeDocument(doc)
	if _, ok := result["cover"]; ok {
		t.Fatal("old media format (src without asset) should be removed")
	}
}

func TestSanitizeDocument_KeepsNewMediaFormat(t *testing.T) {
	doc := map[string]interface{}{
		"cover": map[string]interface{}{
			"_type": "media",
			"asset": map[string]interface{}{"_ref": "media-123"},
			"alt":   "photo",
		},
	}
	result := SanitizeDocument(doc)
	if _, ok := result["cover"]; !ok {
		t.Fatal("new media format (with asset) should be preserved")
	}
}

func TestSanitizeDocument_FiltersNullsInArrays(t *testing.T) {
	doc := map[string]interface{}{
		"items": []interface{}{"a", nil, "b"},
	}
	result := SanitizeDocument(doc)
	items := result["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestSanitizeDocument_RecursiveNested(t *testing.T) {
	doc := map[string]interface{}{
		"meta": map[string]interface{}{
			"seo": map[string]interface{}{
				"title": "Hello",
				"desc":  nil,
			},
		},
	}
	result := SanitizeDocument(doc)
	meta := result["meta"].(map[string]interface{})
	seo := meta["seo"].(map[string]interface{})
	if _, ok := seo["desc"]; ok {
		t.Fatal("nested null should be removed")
	}
	if seo["title"] != "Hello" {
		t.Fatal("nested non-null should be preserved")
	}
}

// --- StripSystemFields ---

func TestStripSystemFields(t *testing.T) {
	doc := map[string]interface{}{
		"id":         "123",
		"_id":        "123",
		"_createdAt": "2024-01-01",
		"_updatedAt": "2024-01-02",
		"_version":   1,
		"_revision":  "abc",
		"_collection": "posts",
		"_type":      "document",
		"title":      "Hello",
		"body":       "World",
	}

	result := StripSystemFields(doc)
	if len(result) != 2 {
		t.Fatalf("expected 2 fields, got %d: %v", len(result), result)
	}
	if result["title"] != "Hello" {
		t.Fatal("title should be preserved")
	}
	if result["body"] != "World" {
		t.Fatal("body should be preserved")
	}
}

func TestStripSystemFields_NoSystemFields(t *testing.T) {
	doc := map[string]interface{}{"title": "Hello"}
	result := StripSystemFields(doc)
	if len(result) != 1 {
		t.Fatalf("expected 1 field, got %d", len(result))
	}
}
