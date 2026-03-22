package backup

import (
	"encoding/json"
	"fmt"
	"strings"
)

// UpdateReferences rewrites document references using idMappings (oldID -> newID).
// Returns the updated document and the number of references rewritten.
func UpdateReferences(doc map[string]interface{}, schema SchemaDefinition, idMappings map[string]string) (map[string]interface{}, int) {
	count := 0
	updateRefsRecursive(doc, schema.Fields, idMappings, &count)
	return doc, count
}

func updateRefsRecursive(obj map[string]interface{}, fields []FieldDefinition, idMappings map[string]string, count *int) {
	for _, field := range fields {
		val, exists := obj[field.Name]
		if !exists || val == nil {
			continue
		}

		switch field.Type {
		case "reference":
			// Reference can be a string ID or an object with _ref
			if refObj, ok := val.(map[string]interface{}); ok {
				if ref, ok := refObj["_ref"].(string); ok {
					if newID, mapped := idMappings[ref]; mapped {
						refObj["_ref"] = newID
						*count++
					}
				}
			} else if ref, ok := val.(string); ok {
				if newID, mapped := idMappings[ref]; mapped {
					obj[field.Name] = newID
					*count++
				}
			}

		case "array":
			if field.Of != nil {
				if arr, ok := val.([]interface{}); ok {
					for i, item := range arr {
						if field.Of.Type == "reference" {
							// Array items are reference values directly
							if refObj, ok := item.(map[string]interface{}); ok {
								if ref, ok := refObj["_ref"].(string); ok {
									if newID, mapped := idMappings[ref]; mapped {
										refObj["_ref"] = newID
										*count++
									}
								}
							} else if ref, ok := item.(string); ok {
								if newID, mapped := idMappings[ref]; mapped {
									arr[i] = newID
									*count++
								}
							}
						} else if itemObj, ok := item.(map[string]interface{}); ok {
							updateRefsRecursive(itemObj, []FieldDefinition{*field.Of}, idMappings, count)
						}
					}
				}
			}

		case "object":
			if len(field.Fields) > 0 {
				if nested, ok := val.(map[string]interface{}); ok {
					updateRefsRecursive(nested, field.Fields, idMappings, count)
				}
			}
		}

		// Check media asset references
		if mediaAssetTypes[field.Type] {
			if mediaObj, ok := val.(map[string]interface{}); ok {
				if asset, ok := mediaObj["asset"].(map[string]interface{}); ok {
					if ref, ok := asset["_ref"].(string); ok {
						if newID, mapped := idMappings[ref]; mapped {
							asset["_ref"] = newID
							*count++
						}
					}
				}
			}
		}
	}
}

// DeepUpdateMediaRefs walks the entire document tree and rewrites any asset._ref
// values found in idMappings. This is a safety net that doesn't require schema info.
func DeepUpdateMediaRefs(doc map[string]interface{}, idMappings map[string]string) int {
	count := 0
	deepUpdateRefs(doc, idMappings, &count)
	return count
}

func deepUpdateRefs(obj interface{}, idMappings map[string]string, count *int) {
	switch v := obj.(type) {
	case map[string]interface{}:
		// Check if this is a media asset reference: {asset: {_ref: "media-xxx"}}
		if asset, ok := v["asset"].(map[string]interface{}); ok {
			if ref, ok := asset["_ref"].(string); ok {
				if newID, mapped := idMappings[ref]; mapped {
					asset["_ref"] = newID
					*count++
				}
			}
		}
		// Check direct _ref (for reference fields)
		if ref, ok := v["_ref"].(string); ok {
			if newID, mapped := idMappings[ref]; mapped {
				v["_ref"] = newID
				*count++
			}
		}
		// Recurse into all values
		for _, val := range v {
			deepUpdateRefs(val, idMappings, count)
		}
	case []interface{}:
		for _, item := range v {
			deepUpdateRefs(item, idMappings, count)
		}
	}
}

// DeepReplaceMediaIDsInStrings walks the entire document and replaces old media IDs
// with new IDs inside string values (e.g. richtext HTML containing /api/media/media-xxx/file).
func DeepReplaceMediaIDsInStrings(doc map[string]interface{}, idMappings map[string]string) int {
	count := 0
	replaceInStrings(doc, idMappings, &count)
	return count
}

func replaceInStrings(obj interface{}, idMappings map[string]string, count *int) {
	switch v := obj.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if s, ok := val.(string); ok {
				replaced := s
				for oldID, newID := range idMappings {
					if oldID != "" && newID != "" && oldID != newID {
						if idx := len(replaced); idx > 0 {
							// Only scan strings that actually contain a media ID
							if containsString(replaced, oldID) {
								replaced = replaceAll(replaced, oldID, newID)
							}
						}
					}
				}
				if replaced != s {
					v[key] = replaced
					*count++
				}
			} else {
				replaceInStrings(val, idMappings, count)
			}
		}
	case []interface{}:
		for i, item := range v {
			if s, ok := item.(string); ok {
				replaced := s
				for oldID, newID := range idMappings {
					if oldID != "" && newID != "" && oldID != newID && containsString(replaced, oldID) {
						replaced = replaceAll(replaced, oldID, newID)
					}
				}
				if replaced != s {
					v[i] = replaced
					*count++
				}
			} else {
				replaceInStrings(item, idMappings, count)
			}
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}

func replaceAll(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

// SanitizeDocument removes null values, empty objects, and old media formats.
func SanitizeDocument(obj map[string]interface{}) map[string]interface{} {
	return sanitizeRecursive(obj)
}

func sanitizeRecursive(obj map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for key, val := range obj {
		if val == nil {
			continue
		}

		switch v := val.(type) {
		case map[string]interface{}:
			sanitized := sanitizeRecursive(v)
			if len(sanitized) == 0 {
				continue
			}
			// Skip old media format (src-based instead of asset._ref)
			if sanitized["_type"] == "media" {
				if _, hasSrc := sanitized["src"]; hasSrc {
					if _, hasAsset := sanitized["asset"]; !hasAsset {
						continue
					}
				}
			}
			result[key] = sanitized

		case []interface{}:
			filtered := make([]interface{}, 0, len(v))
			for _, item := range v {
				if item == nil {
					continue
				}
				if itemObj, ok := item.(map[string]interface{}); ok {
					sanitized := sanitizeRecursive(itemObj)
					if len(sanitized) == 0 {
						continue
					}
					filtered = append(filtered, sanitized)
				} else {
					filtered = append(filtered, item)
				}
			}
			result[key] = filtered

		default:
			result[key] = val
		}
	}

	return result
}

var systemFields = map[string]bool{
	"id": true, "_id": true, "_createdAt": true, "_updatedAt": true,
	"_version": true, "_revision": true, "_collection": true, "_type": true,
}

// StripSystemFields removes internal fields that should not be sent on create.
func StripSystemFields(doc map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(doc))
	for k, v := range doc {
		if !systemFields[k] {
			result[k] = v
		}
	}
	return result
}

// ExtractDocID extracts the document ID from a parsed document, trying "id" then "_id".
func ExtractDocID(doc map[string]interface{}) string {
	if id, ok := doc["id"].(string); ok && id != "" {
		return id
	}
	if id, ok := doc["_id"].(string); ok && id != "" {
		return id
	}
	return ""
}

// ParseSchemas parses schema data, handling multiple API response formats.
func ParseSchemas(data []byte) ([]SchemaDefinition, error) {
	// Try direct []SchemaDefinition
	var schemas []SchemaDefinition
	if err := json.Unmarshal(data, &schemas); err == nil && len(schemas) > 0 {
		for i := range schemas {
			schemas[i].UnmarshalFields()
		}
		return schemas, nil
	}

	// Try {collections: [...]} wrapper
	var wrapper struct {
		Collections []SchemaDefinition `json:"collections"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Collections) > 0 {
		for i := range wrapper.Collections {
			wrapper.Collections[i].UnmarshalFields()
		}
		return wrapper.Collections, nil
	}

	// Try []string (just names)
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("failed to parse schemas: %w", err)
	}

	schemas = make([]SchemaDefinition, len(names))
	for i, name := range names {
		schemas[i] = SchemaDefinition{Name: name}
	}
	return schemas, nil
}

// ParseDocuments parses document list data, trying []map then falling back to {documents: [...]}.
func ParseDocuments(data []byte) []map[string]interface{} {
	var docs []map[string]interface{}
	if json.Unmarshal(data, &docs) == nil {
		return docs
	}

	var wrapper struct {
		Documents []map[string]interface{} `json:"documents"`
	}
	if json.Unmarshal(data, &wrapper) == nil {
		return wrapper.Documents
	}

	return nil
}
