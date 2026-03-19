package backup

import "fmt"

// ValidationError describes a single field validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) String() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult holds the outcome of document validation.
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidateDocument checks a document against a schema definition.
// Validates top-level fields only; nested object/array contents are not checked.
// Returns validation errors for missing required fields and type mismatches.
func ValidateDocument(doc map[string]interface{}, schema SchemaDefinition) ValidationResult {
	result := ValidationResult{Valid: true}

	for _, field := range schema.Fields {
		val, exists := doc[field.Name]

		// Check required fields
		if field.Required && (!exists || val == nil) {
			result.Errors = append(result.Errors, ValidationError{
				Field:   field.Name,
				Message: "required field is missing",
			})
			result.Valid = false
			continue
		}

		if !exists || val == nil {
			continue
		}

		// Basic type checks
		switch field.Type {
		case "string":
			if _, ok := val.(string); !ok {
				result.Errors = append(result.Errors, ValidationError{
					Field:   field.Name,
					Message: fmt.Sprintf("expected string, got %T", val),
				})
				result.Valid = false
			}
		case "number":
			if _, ok := val.(float64); !ok {
				result.Errors = append(result.Errors, ValidationError{
					Field:   field.Name,
					Message: fmt.Sprintf("expected number, got %T", val),
				})
				result.Valid = false
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				result.Errors = append(result.Errors, ValidationError{
					Field:   field.Name,
					Message: fmt.Sprintf("expected boolean, got %T", val),
				})
				result.Valid = false
			}
		case "array":
			if _, ok := val.([]interface{}); !ok {
				result.Errors = append(result.Errors, ValidationError{
					Field:   field.Name,
					Message: fmt.Sprintf("expected array, got %T", val),
				})
				result.Valid = false
			}
		case "object":
			if _, ok := val.(map[string]interface{}); !ok {
				result.Errors = append(result.Errors, ValidationError{
					Field:   field.Name,
					Message: fmt.Sprintf("expected object, got %T", val),
				})
				result.Valid = false
			}
		}
	}

	return result
}
