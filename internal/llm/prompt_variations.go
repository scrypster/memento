// Package llm provides prompt variation utilities for systematic testing and optimization
package llm

import (
	"fmt"
)

// PromptVariation represents a single prompt variation with metadata
type PromptVariation struct {
	ID          int
	Name        string
	Description string
	Generator   func(content string) string
}

// EntityExtractionVariations defines all 25 variations for testing
var EntityExtractionVariations = []PromptVariation{
	{
		ID:          1,
		Name:        "Ultra-Strict JSON",
		Description: "Explicitly defines mandatory fields, types, allowed values, and rejects any deviation",
		Generator: func(content string) string {
			return fmt.Sprintf(`TASK: Extract entities and relationships from the following text.

OUTPUT REQUIREMENT: Return ONLY valid JSON. No markdown. No code blocks. No backticks. No explanations.

ENTITY TYPES (strict):
- "person": A named individual human
- "organization": A company, institution, or group entity
- "tool": Software, library, framework, or technology
- "project": A specific initiative, product, or named work

REQUIRED JSON STRUCTURE:
{
  "entities": [
    {
      "id": "entity_1",
      "name": "string (required, non-empty)",
      "type": "string (must be: person|organization|tool|project)",
      "confidence": "number (0.0-1.0)"
    }
  ],
  "relationships": [
    {
      "source_id": "string (must exist in entities)",
      "target_id": "string (must exist in entities)",
      "type": "string (must be: works_for|created|uses|owns|manages|sibling_of|parent_of|child_of)",
      "confidence": "number (0.0-1.0)"
    }
  ]
}

VALIDATION RULES:
1. Every "id" must be referenced in relationships or omitted from relationships
2. No relationship can reference an entity id that doesn't exist
3. Relationship types are ONLY: works_for|created|uses|owns|manages|sibling_of|parent_of|child_of
4. Confidence values must be numeric between 0.0 and 1.0
5. No extra fields allowed
6. No null values allowed
7. No trailing commas
8. Arrays must contain objects with identical field structure

TEXT TO EXTRACT FROM:
%s

OUTPUT (valid JSON only):`, content)
		},
	},
	{
		ID:          2,
		Name:        "Minimal Concise",
		Description: "Ultra-condensed instructions (30%% shorter) with only essential requirements",
		Generator: func(content string) string {
			return fmt.Sprintf(`Extract entities and relationships. Output only JSON.

Entities: person|organization|tool|project
Relations: works_for|created|uses|owns|manages|sibling_of|parent_of|child_of

Structure:
{"entities":[{"id":"e1","name":"...","type":"person","confidence":0.9}]}
{"relationships":[{"source_id":"e1","target_id":"e2","type":"works_for","confidence":0.8}]}

Text: %s

JSON only (no markdown):`, content)
		},
	},
	{
		ID:          3,
		Name:        "Example-Driven",
		Description: "Leads with detailed worked examples before instructions",
		Generator: func(content string) string {
			return fmt.Sprintf(`EXAMPLE INPUT:
"Alice works at Acme Inc. She uses Python and manages the DataPipeline project."

EXAMPLE OUTPUT:
{
  "entities": [
    {"id": "1", "name": "Alice", "type": "person", "confidence": 0.95},
    {"id": "2", "name": "Acme Inc", "type": "organization", "confidence": 0.95},
    {"id": "3", "name": "Python", "type": "tool", "confidence": 0.95},
    {"id": "4", "name": "DataPipeline", "type": "project", "confidence": 0.95}
  ],
  "relationships": [
    {"source_id": "1", "target_id": "2", "type": "works_for", "confidence": 0.95},
    {"source_id": "1", "target_id": "3", "type": "uses", "confidence": 0.95},
    {"source_id": "1", "target_id": "4", "type": "manages", "confidence": 0.95}
  ]
}

NOW EXTRACT FROM THIS TEXT (same JSON format):
%s

JSON OUTPUT (no markdown, no backticks):`, content)
		},
	},
	{
		ID:          5,
		Name:        "Aggressive JSON Only",
		Description: "Repeats 'ONLY JSON' requirement multiple times with visual emphasis",
		Generator: func(content string) string {
			return fmt.Sprintf(`======== JSON ONLY ========
RETURN ONLY JSON.
NOT MARKDOWN.
NOT CODE BLOCKS.
NOT BACKTICKS.
NOT EXPLANATIONS.
ONLY: {valid JSON object}

Entity types: person, organization, tool, project
Relationship types: works_for, created, uses, owns, manages, sibling_of, parent_of, child_of

Text: %s

======== OUTPUT JSON ONLY ========`, content)
		},
	},
}

// GetVariation returns a prompt variation by ID
func GetVariation(id int) *PromptVariation {
	for _, v := range EntityExtractionVariations {
		if v.ID == id {
			return &v
		}
	}
	return nil
}

// GetVariationNames returns all variation names
func GetVariationNames() []string {
	names := make([]string, len(EntityExtractionVariations))
	for i, v := range EntityExtractionVariations {
		names[i] = v.Name
	}
	return names
}
