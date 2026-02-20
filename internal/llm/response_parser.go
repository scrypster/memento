package llm

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/scrypster/memento/pkg/types"
)

// EntityResponse represents a single entity extracted from LLM response
type EntityResponse struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description string  `json:"description,omitempty"`
	Confidence  float64 `json:"confidence"`
}

// EntityExtractionResponse represents the complete entity extraction response
type EntityExtractionResponse struct {
	Entities []EntityResponse `json:"entities"`
}

// RelationshipResponse represents a single relationship extracted from LLM response
type RelationshipResponse struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}

// RelationshipExtractionResponse represents the complete relationship extraction response
type RelationshipExtractionResponse struct {
	Relationships []RelationshipResponse `json:"relationships"`
}

// SkippedTypeInfo records a single entity or relationship that was skipped
// because its type was not in the allowed list.
type SkippedTypeInfo struct {
	Domain   string // "entity" or "relationship"
	TypeName string // the unrecognized type string
	Name     string // example name (entity name or "from→to" for relationships)
}

// ClassificationResponse represents a single classification result
type ClassificationResponse struct {
	MemoryType     string   `json:"memory_type"`
	Category       string   `json:"category"`
	Classification string   `json:"classification,omitempty"` // Specific classification within category
	Subcategory    *string  `json:"subcategory,omitempty"`
	Priority       string   `json:"priority"`
	ContextLabels  []string `json:"context_labels"`
	Tags           []string `json:"tags"`
	Confidence     float64  `json:"confidence"`
}

// SummarizationResponse represents the summarization response
type SummarizationResponse struct {
	Summary   string   `json:"summary"`
	KeyPoints []string `json:"key_points"`
}

// KeywordExtractionResponse represents the keyword extraction response
type KeywordExtractionResponse struct {
	Keywords []string `json:"keywords"`
}

// extractJSON extracts the first valid JSON object from a string that may contain extra text.
// This handles cases where LLMs add explanations before/after the JSON despite instructions.
func extractJSON(text string) string {
	// Remove common markdown code block markers
	text = strings.ReplaceAll(text, "```json", "")
	text = strings.ReplaceAll(text, "```", "")
	text = strings.TrimSpace(text)

	// Try to find JSON object boundaries
	start := strings.Index(text, "{")
	if start == -1 {
		return text // No JSON found, return as-is and let parser fail
	}

	// Find the matching closing brace
	braceCount := 0
	inString := false
	escape := false

	for i := start; i < len(text); i++ {
		char := text[i]

		// Handle string escaping
		if escape {
			escape = false
			continue
		}
		if char == '\\' {
			escape = true
			continue
		}

		// Track if we're inside a string
		if char == '"' {
			inString = !inString
			continue
		}

		// Only count braces outside of strings
		if !inString {
			switch char {
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					// Found complete JSON object, return it
					return text[start : i+1]
				}
			}
		}
	}

	return text // No complete JSON found, return as-is
}

// ParseEntityResponse parses entity extraction JSON and filters out invalid entries.
// Invalid entity types or confidence scores are skipped rather than failing the entire batch.
// Only returns an error if the JSON itself is malformed.
//
// Parameters:
//   - jsonStr: JSON string returned by the LLM (may contain extra text)
//
// Returns:
//   - Slice of valid EntityResponse objects (may be shorter than LLM output)
//   - Error only if the JSON itself is malformed
func ParseEntityResponse(jsonStr string) ([]EntityResponse, error) {
	valid, _, err := ParseEntityResponseDetailed(jsonStr)
	return valid, err
}

// ParseRelationshipResponse parses relationship extraction JSON and filters out invalid entries.
// Invalid relationship types or confidence scores are skipped (with a log-worthy reason returned
// in the second value) rather than failing the entire batch.
//
// Parameters:
//   - jsonStr: JSON string returned by the LLM
//
// Returns:
//   - Slice of valid RelationshipResponse objects (may be shorter than LLM output)
//   - Error only if the JSON itself is malformed
func ParseRelationshipResponse(jsonStr string) ([]RelationshipResponse, error) {
	valid, _, err := ParseRelationshipResponseDetailed(jsonStr)
	return valid, err
}

// ParseSummarizationResponse parses summarization JSON response.
// It returns an error if the JSON is malformed.
//
// Parameters:
//   - jsonStr: JSON string returned by the LLM
//
// Returns:
//   - SummarizationResponse object
//   - Error if parsing fails
func ParseSummarizationResponse(jsonStr string) (*SummarizationResponse, error) {
	// Extract just the JSON part, ignoring any extra text
	cleanJSON := extractJSON(jsonStr)

	var response SummarizationResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, fmt.Errorf("failed to parse summarization JSON: %w", err)
	}

	return &response, nil
}

// ParseKeywordResponse parses keyword extraction JSON response.
// It returns an error if the JSON is malformed.
//
// Parameters:
//   - jsonStr: JSON string returned by the LLM
//
// Returns:
//   - Slice of keyword strings
//   - Error if parsing fails
func ParseKeywordResponse(jsonStr string) ([]string, error) {
	var response KeywordExtractionResponse
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		return nil, fmt.Errorf("failed to parse keyword JSON: %w", err)
	}

	return response.Keywords, nil
}

// ParseClassificationResponse parses classification extraction JSON response.
// It validates memory type, category, priority, and other fields according to allowed values.
//
// Parameters:
//   - jsonStr: JSON string returned by the LLM
//
// Returns:
//   - ClassificationResponse object
//   - Error if parsing or validation fails
func ParseClassificationResponse(jsonStr string) (*ClassificationResponse, error) {
	cleanJSON := extractJSON(jsonStr)

	var response ClassificationResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, fmt.Errorf("failed to parse classification JSON: %w", err)
	}

	// Validate memory type
	if !types.IsValidMemoryType(response.MemoryType) {
		return nil, fmt.Errorf("invalid memory type: %s (must be one of: decision, process, concept, event, person, system, rule, project, epic, phase, milestone, task, step)", response.MemoryType)
	}

	// Validate category (allow custom categories from connections, but provide defaults)
	validCategories := map[string]bool{
		"Architecture":              true,
		"Security":                  true,
		"Performance":               true,
		"Technical":                 true,
		"Business":                  true,
		"Operations":                true,
		"Documentation":             true,
		"Meeting":                   true,
		"Decision":                  true,
		"Software Development":      true,
		"Project Management":        true,
		"Business & Operations":     true,
		"Research & Learning":       true,
		"Personal Assistant":        true,
		"Communication & Collaboration": true,
		"Other":                     true,
	}
	if !validCategories[response.Category] {
		// Allow custom categories (they may be set per connection)
		// Only warn if it's completely invalid, don't fail
		_ = response.Category
	}

	// Validate priority
	validPriorities := map[string]bool{
		"Critical": true,
		"High":     true,
		"Medium":   true,
		"Low":      true,
	}
	if !validPriorities[response.Priority] {
		return nil, fmt.Errorf("invalid priority: %s (must be one of: Critical, High, Medium, Low)", response.Priority)
	}

	// Validate confidence
	if response.Confidence < 0.0 || response.Confidence > 1.0 {
		return nil, fmt.Errorf("invalid confidence score: %f (must be 0.0-1.0)", response.Confidence)
	}

	return &response, nil
}

// ValidateEntityTypes ensures all entity types are valid according to pkg/types constants.
// It returns an error if any entity has an invalid type, with details about which
// entity and type failed validation.
//
// Parameters:
//   - entities: Slice of EntityResponse objects to validate
//
// Returns:
//   - Error if any entity type is invalid, nil otherwise
func ValidateEntityTypes(entities []EntityResponse) error {
	for i, entity := range entities {
		if !types.IsValidEntityType(entity.Type) {
			return fmt.Errorf("entity %d (%s) has invalid entity type: %s (must be one of: person, organization, project, location, event, document, note, file, url, email, message, concept, task, repository, code_snippet, api, database, server, tool, framework, language, library)", i, entity.Name, entity.Type)
		}
	}
	return nil
}

// ValidateRelationshipTypes filters relationships to only those with valid types.
// Instead of returning an error on the first invalid type, it returns only the valid subset.
// This allows partial results to be preserved when the LLM uses an unknown type.
//
// Parameters:
//   - rels: Slice of RelationshipResponse objects to filter
//
// Returns:
//   - Slice containing only relationships with valid types
func ValidateRelationshipTypes(rels []RelationshipResponse) []RelationshipResponse {
	valid := make([]RelationshipResponse, 0, len(rels))
	for _, rel := range rels {
		if types.IsValidRelationshipType(rel.Type) {
			valid = append(valid, rel)
		}
	}
	return valid
}

// ParseEntityResponseWithTypes parses entity extraction JSON and filters out entries
// whose type is not in the provided allowed list (system defaults + custom types).
// Invalid entities are skipped rather than failing the entire batch.
// Use this when custom entity types may be present.
func ParseEntityResponseWithTypes(jsonStr string, allowedTypes []string) ([]EntityResponse, error) {
	valid, _, err := ParseEntityResponseWithTypesDetailed(jsonStr, allowedTypes)
	return valid, err
}

// ParseRelationshipResponseWithTypes parses relationship extraction JSON and filters out entries
// whose type is not in the provided allowed list (system defaults + custom types).
// Invalid relationships are skipped rather than failing the entire batch.
func ParseRelationshipResponseWithTypes(jsonStr string, allowedTypes []string) ([]RelationshipResponse, error) {
	valid, _, err := ParseRelationshipResponseWithTypesDetailed(jsonStr, allowedTypes)
	return valid, err
}

// ParseEntityResponseDetailed parses entity extraction JSON and returns both
// valid entities and a list of skipped entries (unknown type or bad confidence).
func ParseEntityResponseDetailed(jsonStr string) ([]EntityResponse, []SkippedTypeInfo, error) {
	cleanJSON := extractJSON(jsonStr)
	var response EntityExtractionResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, nil, fmt.Errorf("failed to parse entity JSON: %w", err)
	}
	var valid []EntityResponse
	var skipped []SkippedTypeInfo
	for _, entity := range response.Entities {
		if !types.IsValidEntityType(entity.Type) {
			log.Printf("response_parser: skipping entity %q with unknown type %q", entity.Name, entity.Type)
			skipped = append(skipped, SkippedTypeInfo{Domain: "entity", TypeName: entity.Type, Name: entity.Name})
			continue
		}
		if entity.Confidence < 0.0 || entity.Confidence > 1.0 {
			log.Printf("response_parser: skipping entity %q with invalid confidence %f", entity.Name, entity.Confidence)
			continue
		}
		valid = append(valid, entity)
	}
	return valid, skipped, nil
}

// ParseEntityResponseWithTypesDetailed parses entity extraction JSON against a custom allowed
// type list and returns both valid entities and skipped entries.
func ParseEntityResponseWithTypesDetailed(jsonStr string, allowedTypes []string) ([]EntityResponse, []SkippedTypeInfo, error) {
	if len(allowedTypes) == 0 {
		return ParseEntityResponseDetailed(jsonStr)
	}
	cleanJSON := extractJSON(jsonStr)
	var response EntityExtractionResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, nil, fmt.Errorf("failed to parse entity JSON: %w", err)
	}
	allowed := make(map[string]bool, len(allowedTypes))
	for _, t := range allowedTypes {
		allowed[t] = true
	}
	var valid []EntityResponse
	var skipped []SkippedTypeInfo
	for _, entity := range response.Entities {
		if !allowed[entity.Type] {
			log.Printf("response_parser: skipping entity %q with unknown type %q", entity.Name, entity.Type)
			skipped = append(skipped, SkippedTypeInfo{Domain: "entity", TypeName: entity.Type, Name: entity.Name})
			continue
		}
		if entity.Confidence < 0.0 || entity.Confidence > 1.0 {
			log.Printf("response_parser: skipping entity %q with invalid confidence %f", entity.Name, entity.Confidence)
			continue
		}
		valid = append(valid, entity)
	}
	return valid, skipped, nil
}

// ParseRelationshipResponseDetailed parses relationship extraction JSON and returns both
// valid relationships and a list of skipped entries (unknown type or bad confidence).
func ParseRelationshipResponseDetailed(jsonStr string) ([]RelationshipResponse, []SkippedTypeInfo, error) {
	cleanJSON := extractJSON(jsonStr)
	var response RelationshipExtractionResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, nil, fmt.Errorf("failed to parse relationship JSON: %w", err)
	}
	var valid []RelationshipResponse
	var skipped []SkippedTypeInfo
	for _, rel := range response.Relationships {
		if !types.IsValidRelationshipType(rel.Type) {
			log.Printf("response_parser: skipping relationship %q→%q with unknown type %q", rel.From, rel.To, rel.Type)
			skipped = append(skipped, SkippedTypeInfo{Domain: "relationship", TypeName: rel.Type, Name: rel.From + "→" + rel.To})
			continue
		}
		if rel.Confidence < 0.0 || rel.Confidence > 1.0 {
			continue
		}
		valid = append(valid, rel)
	}
	return valid, skipped, nil
}

// ParseRelationshipResponseWithTypesDetailed parses relationship extraction JSON against a custom
// allowed type list and returns both valid relationships and skipped entries.
func ParseRelationshipResponseWithTypesDetailed(jsonStr string, allowedTypes []string) ([]RelationshipResponse, []SkippedTypeInfo, error) {
	if len(allowedTypes) == 0 {
		return ParseRelationshipResponseDetailed(jsonStr)
	}
	cleanJSON := extractJSON(jsonStr)
	var response RelationshipExtractionResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, nil, fmt.Errorf("failed to parse relationship JSON: %w", err)
	}
	allowed := make(map[string]bool, len(allowedTypes))
	for _, t := range allowedTypes {
		allowed[t] = true
	}
	var valid []RelationshipResponse
	var skipped []SkippedTypeInfo
	for _, rel := range response.Relationships {
		if !allowed[rel.Type] {
			log.Printf("response_parser: skipping relationship %q→%q with unknown type %q", rel.From, rel.To, rel.Type)
			skipped = append(skipped, SkippedTypeInfo{Domain: "relationship", TypeName: rel.Type, Name: rel.From + "→" + rel.To})
			continue
		}
		if rel.Confidence < 0.0 || rel.Confidence > 1.0 {
			continue
		}
		valid = append(valid, rel)
	}
	return valid, skipped, nil
}

// ParseClassificationResponseWithSettings parses classification extraction JSON and validates
// memory_type against the merged allowed types from settings.
// Category validation is lenient (custom categories are always allowed).
func ParseClassificationResponseWithSettings(jsonStr string, settings *types.SettingsResponse) (*ClassificationResponse, error) {
	if settings == nil {
		return ParseClassificationResponse(jsonStr)
	}

	cleanJSON := extractJSON(jsonStr)

	var response ClassificationResponse
	if err := json.Unmarshal([]byte(cleanJSON), &response); err != nil {
		return nil, fmt.Errorf("failed to parse classification JSON: %w", err)
	}

	// Validate memory type against merged list
	if len(settings.AllMemoryTypes) > 0 {
		allowed := make(map[string]bool, len(settings.AllMemoryTypes))
		for _, t := range settings.AllMemoryTypes {
			allowed[t] = true
		}
		if !allowed[response.MemoryType] {
			return nil, fmt.Errorf("invalid memory type: %s (must be one of: %s)", response.MemoryType, strings.Join(settings.AllMemoryTypes, ", "))
		}
	} else if !types.IsValidMemoryType(response.MemoryType) {
		return nil, fmt.Errorf("invalid memory type: %s", response.MemoryType)
	}

	// Validate priority
	validPriorities := map[string]bool{
		"Critical": true, "High": true, "Medium": true, "Low": true,
	}
	if !validPriorities[response.Priority] {
		return nil, fmt.Errorf("invalid priority: %s (must be one of: Critical, High, Medium, Low)", response.Priority)
	}

	// Validate confidence
	if response.Confidence < 0.0 || response.Confidence > 1.0 {
		return nil, fmt.Errorf("invalid confidence score: %f (must be 0.0-1.0)", response.Confidence)
	}

	return &response, nil
}
