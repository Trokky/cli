package backup

import (
	"encoding/json"
	"sort"
	"testing"
)

// --- FieldTargets ---

func TestFieldTargets_SingleString(t *testing.T) {
	field := FieldDefinition{
		Type: "reference",
		To:   json.RawMessage(`"authors"`),
	}
	targets := field.FieldTargets()
	if len(targets) != 1 || targets[0] != "authors" {
		t.Fatalf("expected [authors], got %v", targets)
	}
}

func TestFieldTargets_StringArray(t *testing.T) {
	field := FieldDefinition{
		Type: "reference",
		To:   json.RawMessage(`["authors","categories"]`),
	}
	targets := field.FieldTargets()
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
}

func TestFieldTargets_Nil(t *testing.T) {
	field := FieldDefinition{Type: "string"}
	targets := field.FieldTargets()
	if targets != nil {
		t.Fatalf("expected nil, got %v", targets)
	}
}

// --- BuildDependencyGraph ---

func TestBuildDependencyGraph_NoDeps(t *testing.T) {
	schemas := []SchemaDefinition{
		{Name: "posts", Fields: []FieldDefinition{
			{Name: "title", Type: "string"},
		}},
	}

	graph := BuildDependencyGraph(schemas)
	if len(graph["posts"]) != 0 {
		t.Fatalf("expected no deps, got %v", graph["posts"])
	}
}

func TestBuildDependencyGraph_ReferenceField(t *testing.T) {
	schemas := []SchemaDefinition{
		{Name: "posts", Fields: []FieldDefinition{
			{Name: "author", Type: "reference", To: json.RawMessage(`"authors"`)},
		}},
		{Name: "authors", Fields: []FieldDefinition{
			{Name: "name", Type: "string"},
		}},
	}

	graph := BuildDependencyGraph(schemas)
	deps := graph["posts"]
	if len(deps) != 1 || deps[0] != "authors" {
		t.Fatalf("expected [authors], got %v", deps)
	}
	if len(graph["authors"]) != 0 {
		t.Fatalf("authors should have no deps, got %v", graph["authors"])
	}
}

func TestBuildDependencyGraph_MediaField(t *testing.T) {
	schemas := []SchemaDefinition{
		{Name: "posts", Fields: []FieldDefinition{
			{Name: "cover", Type: "image"},
		}},
	}

	graph := BuildDependencyGraph(schemas)
	deps := graph["posts"]
	if len(deps) != 1 || deps[0] != "media" {
		t.Fatalf("expected [media], got %v", deps)
	}
}

func TestBuildDependencyGraph_ArrayOfReferences(t *testing.T) {
	schemas := []SchemaDefinition{
		{Name: "posts", Fields: []FieldDefinition{
			{Name: "tags", Type: "array", Of: &FieldDefinition{
				Type: "reference", To: json.RawMessage(`"tags"`),
			}},
		}},
	}

	graph := BuildDependencyGraph(schemas)
	deps := graph["posts"]
	if len(deps) != 1 || deps[0] != "tags" {
		t.Fatalf("expected [tags], got %v", deps)
	}
}

func TestBuildDependencyGraph_NestedObjectReference(t *testing.T) {
	schemas := []SchemaDefinition{
		{Name: "posts", Fields: []FieldDefinition{
			{Name: "meta", Type: "object", Fields: []FieldDefinition{
				{Name: "category", Type: "reference", To: json.RawMessage(`"categories"`)},
			}},
		}},
	}

	graph := BuildDependencyGraph(schemas)
	deps := graph["posts"]
	if len(deps) != 1 || deps[0] != "categories" {
		t.Fatalf("expected [categories], got %v", deps)
	}
}

func TestBuildDependencyGraph_MultipleRefTargets(t *testing.T) {
	schemas := []SchemaDefinition{
		{Name: "posts", Fields: []FieldDefinition{
			{Name: "related", Type: "reference", To: json.RawMessage(`["pages","articles"]`)},
		}},
	}

	graph := BuildDependencyGraph(schemas)
	deps := graph["posts"]
	sort.Strings(deps)
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %v", deps)
	}
	if deps[0] != "articles" || deps[1] != "pages" {
		t.Fatalf("expected [articles pages], got %v", deps)
	}
}

func TestBuildDependencyGraph_AllMediaTypes(t *testing.T) {
	for _, mediaType := range []string{"media", "image", "video", "audio", "file"} {
		schemas := []SchemaDefinition{
			{Name: "test", Fields: []FieldDefinition{
				{Name: "f", Type: mediaType},
			}},
		}

		graph := BuildDependencyGraph(schemas)
		deps := graph["test"]
		if len(deps) != 1 || deps[0] != "media" {
			t.Fatalf("type %q: expected [media], got %v", mediaType, deps)
		}
	}
}

// --- GetRestoreOrder ---

func TestGetRestoreOrder_NoDeps(t *testing.T) {
	graph := map[string][]string{
		"posts":  {},
		"pages":  {},
	}

	order := GetRestoreOrder(graph)
	if len(order) != 2 {
		t.Fatalf("expected 2, got %d", len(order))
	}
}

func TestGetRestoreOrder_Linear(t *testing.T) {
	graph := map[string][]string{
		"posts":   {"authors"},
		"authors": {},
	}

	order := GetRestoreOrder(graph)
	authorsIdx := -1
	postsIdx := -1
	for i, name := range order {
		if name == "authors" {
			authorsIdx = i
		}
		if name == "posts" {
			postsIdx = i
		}
	}
	if authorsIdx >= postsIdx {
		t.Fatalf("authors should come before posts, got order %v", order)
	}
}

func TestGetRestoreOrder_Diamond(t *testing.T) {
	// D depends on B and C, both depend on A
	graph := map[string][]string{
		"A": {},
		"B": {"A"},
		"C": {"A"},
		"D": {"B", "C"},
	}

	order := GetRestoreOrder(graph)
	indexOf := make(map[string]int)
	for i, name := range order {
		indexOf[name] = i
	}

	if indexOf["A"] >= indexOf["B"] {
		t.Fatal("A should come before B")
	}
	if indexOf["A"] >= indexOf["C"] {
		t.Fatal("A should come before C")
	}
	if indexOf["B"] >= indexOf["D"] {
		t.Fatal("B should come before D")
	}
	if indexOf["C"] >= indexOf["D"] {
		t.Fatal("C should come before D")
	}
}

func TestGetRestoreOrder_SkipsExternalDeps(t *testing.T) {
	// "posts" depends on "media" which is not in the graph
	graph := map[string][]string{
		"posts": {"media"},
	}

	order := GetRestoreOrder(graph)
	if len(order) != 1 || order[0] != "posts" {
		t.Fatalf("expected [posts], got %v", order)
	}
}

// --- ValidateSchemaCompatibility ---

func TestValidateSchemaCompatibility_AllPresent(t *testing.T) {
	source := []SchemaDefinition{{Name: "posts"}, {Name: "pages"}}
	target := []SchemaDefinition{{Name: "posts"}, {Name: "pages"}, {Name: "extra"}}

	result := ValidateSchemaCompatibility(source, target)
	if !result.Compatible {
		t.Fatalf("expected compatible, got errors: %v", result.Errors)
	}
}

func TestValidateSchemaCompatibility_MissingCollection(t *testing.T) {
	source := []SchemaDefinition{{Name: "posts"}, {Name: "missing"}}
	target := []SchemaDefinition{{Name: "posts"}}

	result := ValidateSchemaCompatibility(source, target)
	if result.Compatible {
		t.Fatal("expected incompatible")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestValidateSchemaCompatibility_Empty(t *testing.T) {
	result := ValidateSchemaCompatibility(nil, nil)
	if !result.Compatible {
		t.Fatal("empty schemas should be compatible")
	}
}
