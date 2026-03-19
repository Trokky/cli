package backup

// mediaAssetTypes are field types that reference media assets.
var mediaAssetTypes = map[string]bool{
	"media": true, "image": true, "video": true, "audio": true, "file": true,
}

// BuildDependencyGraph builds a map of collection -> dependencies from schemas.
func BuildDependencyGraph(schemas []SchemaDefinition) map[string][]string {
	graph := make(map[string][]string)

	for _, schema := range schemas {
		deps := make(map[string]bool)
		scanFieldsForDeps(schema.Fields, deps)

		depList := make([]string, 0, len(deps))
		for d := range deps {
			depList = append(depList, d)
		}
		graph[schema.Name] = depList
	}

	return graph
}

func scanFieldsForDeps(fields []FieldDefinition, deps map[string]bool) {
	for _, field := range fields {
		// Reference fields
		if field.Type == "reference" {
			for _, target := range field.FieldTargets() {
				deps[target] = true
			}
		}

		// Media asset fields
		if mediaAssetTypes[field.Type] {
			deps["media"] = true
		}

		// Array fields — recurse into "of"
		if field.Type == "array" && field.Of != nil {
			scanFieldsForDeps([]FieldDefinition{*field.Of}, deps)
		}

		// Object fields — recurse into nested fields
		if field.Type == "object" && len(field.Fields) > 0 {
			scanFieldsForDeps(field.Fields, deps)
		}
	}
}

// GetRestoreOrder returns a topologically sorted list of collections.
// Collections with no dependencies come first.
func GetRestoreOrder(graph map[string][]string) []string {
	visited := make(map[string]bool)
	var order []string

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		for _, dep := range graph[name] {
			if _, inGraph := graph[dep]; inGraph {
				visit(dep)
			}
		}

		order = append(order, name)
	}

	for name := range graph {
		visit(name)
	}

	return order
}

// SchemaCompatibility holds the result of schema validation.
type SchemaCompatibility struct {
	Compatible bool
	Errors     []string
	Warnings   []string
}

// ValidateSchemaCompatibility checks if source schemas are compatible with target schemas.
func ValidateSchemaCompatibility(source, target []SchemaDefinition) SchemaCompatibility {
	result := SchemaCompatibility{Compatible: true}

	targetMap := make(map[string]SchemaDefinition)
	for _, s := range target {
		targetMap[s.Name] = s
	}

	for _, src := range source {
		if _, ok := targetMap[src.Name]; !ok {
			result.Errors = append(result.Errors, "collection '"+src.Name+"' does not exist in target instance")
			result.Compatible = false
		}
	}

	return result
}
