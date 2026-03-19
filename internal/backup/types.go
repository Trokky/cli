package backup

import "encoding/json"

// ManifestVersion is the backup format version.
const ManifestVersion = "2.0"

// BackupManifest contains all metadata needed for restore.
type BackupManifest struct {
	Version         string                   `json:"version"`
	Timestamp       string                   `json:"timestamp"`
	Source          BackupSource             `json:"source"`
	Schemas         []SchemaDefinition       `json:"schemas"`
	DependencyGraph map[string][]string      `json:"dependencyGraph"`
	RestoreOrder    []string                 `json:"restoreOrder"`
	MediaIndex      map[string]MediaFileInfo `json:"mediaIndex"`
	Statistics      BackupStatistics         `json:"statistics"`
}

type BackupSource struct {
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
}

// SchemaDefinition represents a collection schema from the API.
type SchemaDefinition struct {
	Name        string            `json:"name"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"` // "document" or "singleton"
	Singleton   bool              `json:"singleton,omitempty"`
	Fields      []FieldDefinition `json:"fields"`
}

// FieldDefinition describes a field in a schema.
type FieldDefinition struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Title      string            `json:"title,omitempty"`
	Required   bool              `json:"required,omitempty"`
	To         json.RawMessage   `json:"to,omitempty"`    // string or []string for references
	Of         *FieldDefinition  `json:"of,omitempty"`    // for array fields
	Fields     []FieldDefinition `json:"fields,omitempty"` // for object fields
	Hidden     bool              `json:"hidden,omitempty"`
	ReadOnly   bool              `json:"readOnly,omitempty"`
}

// FieldTargets returns the reference target collections for a field.
func (f FieldDefinition) FieldTargets() []string {
	if f.To == nil {
		return nil
	}

	// Try single string
	var single string
	if json.Unmarshal(f.To, &single) == nil {
		return []string{single}
	}

	// Try string array
	var multi []string
	if json.Unmarshal(f.To, &multi) == nil {
		return multi
	}

	return nil
}

// MediaFileInfo describes a media file in the backup.
type MediaFileInfo struct {
	Filename string                 `json:"filename"`
	MimeType string                 `json:"mimeType"`
	Size     int64                  `json:"size"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// BackupStatistics holds summary stats for a backup.
type BackupStatistics struct {
	TotalDocuments int            `json:"totalDocuments"`
	TotalMedia     int            `json:"totalMedia"`
	Collections    map[string]int `json:"collections"`
	BackupSizeBytes int64         `json:"backupSizeBytes"`
}
