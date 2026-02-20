// Package llm provides LLM integration for entity extraction, relationship extraction,
// and content summarization. It includes strict JSON-only prompt templates and response
// parsers that work with Ollama, OpenAI, and Anthropic models.
package llm

import (
	"fmt"
	"strings"

	"github.com/scrypster/memento/pkg/types"
)

// systemEntityTypeDescriptions maps system entity type IDs to brief descriptions for prompts.
var systemEntityTypeDescriptions = map[string]string{
	"person":       "Individual human",
	"organization": "Company, institution, or group",
	"project":      "Named initiative, product, or work",
	"location":     "Place, city, country, or region",
	"event":        "Meeting, incident, or occurrence",
	"document":     "Written document or specification",
	"note":         "Notes or informal content",
	"file":         "File or attachment",
	"url":          "Web link or URL",
	"email":        "Email address",
	"message":      "Message or communication",
	"concept":      "Idea, principle, or theory",
	"task":         "Work item or to-do",
	"repository":   "Code repository",
	"code_snippet": "Code fragment",
	"api":          "API or endpoint",
	"database":     "Database or data store",
	"server":       "Server or host",
	"tool":         "Software tool or utility",
	"framework":    "Framework or platform",
	"language":     "Programming language",
	"library":      "Software library or package",
}

// EntityExtractionPrompt generates a strict JSON-only prompt for entity extraction.
// VARIATION #1: Ultra-Strict JSON (PROVEN BEST)
// The prompt instructs the LLM to extract entities from content and return them
// as a JSON object with an "entities" array containing name, type, description, and confidence fields.
//
// Parameters:
//   - content: The text content to extract entities from
//
// Returns:
//   - A prompt string that will elicit JSON-only responses from the LLM
func EntityExtractionPrompt(content string) string {
	return fmt.Sprintf(`TASK: Extract entities from text.
OUTPUT: ONLY valid JSON. NO markdown. NO code blocks. NO backticks. NO ARRAY - MUST BE OBJECT.

ENTITY TYPES (ONLY these 4):
- person: Individual human
- organization: Company/institution/group entity
- tool: Software, library, framework, technology
- project: Specific initiative/product/named work

REQUIRED JSON STRUCTURE:
Your response MUST start with { and end with }
Your response MUST have an "entities" key with an array value
Each entity MUST have: name, type, description, confidence

Example structure (EXACT FORMAT REQUIRED):
{
  "entities": [
    {"name":"Alice","type":"person","description":"Works at Google","confidence":0.95},
    {"name":"Google","type":"organization","description":"Tech company","confidence":0.95}
  ]
}

VALIDATION (STRICT):
1. Start with { - End with }
2. "entities" key must be present
3. "entities" value must be an array [...]
4. Each entity is an object with: name, type, description, confidence
5. No extra fields - only these 4 per entity
6. No null values
7. No trailing commas
8. Valid JSON syntax
9. Types EXACTLY: person|organization|tool|project
10. Confidence 0.0-1.0

TEXT TO EXTRACT FROM:
%s

RESPOND WITH ONLY THIS JSON STRUCTURE (nothing else):
{"entities":[{"name":"X","type":"person","description":"...","confidence":0.85}]}`, content)
}

// RelationshipExtractionPrompt generates a strict JSON-only prompt for relationship extraction.
// The prompt instructs the LLM to identify relationships between entities and return them
// as a JSON array with from, to, type, and confidence fields.
//
// Parameters:
//   - content: The text content to analyze for relationships
//   - entities: List of entities to find relationships between
//
// Returns:
//   - A prompt string that will elicit JSON-only responses from the LLM
func RelationshipExtractionPrompt(content string, entities []types.Entity) string {
	// Build entity list for the prompt
	var entityList strings.Builder
	for i, entity := range entities {
		fmt.Fprintf(&entityList, "- %s (%s)\n", entity.Name, entity.Type)
		if i >= 50 { // Limit to first 50 entities to avoid token limits
			fmt.Fprintf(&entityList, "... and %d more entities\n", len(entities)-50)
			break
		}
	}

	return fmt.Sprintf(`Find relationships between these entities. Return ONLY valid JSON, no markdown, no code blocks, no explanation.

BIDIRECTIONAL (use ONE direction, system stores both):
- married_to, colleague_of, works_with, friend_of, knows, sibling_of, partners_with

UNIDIRECTIONAL (pick the correct direction):
- employed_by / employs
- manages / managed_by
- reports_to
- leads / led_by
- member_of / has_member
- owns / owned_by
- founded / founded_by
- creates / created_by
- provides / provided_by
- contributes_to
- parent_of / child_of
- contains / belongs_to
- depends_on / required_by
- blocks / blocked_by
- works_on, uses, used_by
- implements, addresses, supersedes, references, documents
- relates_to

RULES:
1. Match entity names EXACTLY as listed
2. Confidence: 0.7-0.99
3. If no relationships found, return {"relationships":[]}
4. Use ONLY types from the lists above

Entities (use exact names):
%s

Content to analyze:
%s

Return ONLY JSON object, nothing else, no markdown:
{"relationships":[{"from":"X","to":"Y","type":"...","confidence":0.85},...]}`, entityList.String(), content)
}

// SummarizationPrompt generates a strict JSON-only prompt for content summarization.
// The prompt instructs the LLM to create a concise summary and extract key points
// from the content.
//
// Parameters:
//   - content: The text content to summarize
//
// Returns:
//   - A prompt string that will elicit JSON-only responses from the LLM
func SummarizationPrompt(content string) string {
	return fmt.Sprintf(`Summarize content. Return ONLY valid JSON, no markdown, no code blocks, no explanation.

Provide:
- summary: 2-3 sentence concise summary
- key_points: array of 3-5 key points

Content:
%s

Return ONLY JSON object, nothing else, no markdown:
{"summary":"...","key_points":["...","..."]}`, content)
}

// KeywordExtractionPrompt generates a strict JSON-only prompt for keyword extraction.
// The prompt instructs the LLM to extract important keywords and phrases from the content.
//
// Parameters:
//   - content: The text content to extract keywords from
//
// Returns:
//   - A prompt string that will elicit JSON-only responses from the LLM
func KeywordExtractionPrompt(content string) string {
	return fmt.Sprintf(`Extract keywords. Return ONLY valid JSON, no markdown, no code blocks, no explanation.

Extract 5-10 important keywords or phrases.

Content:
%s

Return ONLY JSON object, nothing else, no markdown:
{"keywords":["...",...]}}`, content)
}

// ClassificationExtractionPrompt generates a strict JSON-only prompt for content classification.
// The prompt instructs the LLM to classify content into categories, priorities, and context labels.
//
// Parameters:
//   - content: The text content to classify
//
// Returns:
//   - A prompt string that will elicit JSON-only responses from the LLM
func ClassificationExtractionPrompt(content string) string {
	return fmt.Sprintf(`TASK: Classify content by memory type, category, priority, and context.
OUTPUT: ONLY valid JSON. NO markdown. NO code blocks. NO backticks. NO ARRAY - MUST BE OBJECT.

MEMORY TYPE (what kind of memory is this?):
- decision: Important choice or decision made
- process: Step-by-step procedure or workflow
- concept: Idea, principle, or theory
- event: Meeting, incident, or occurrence
- person: Information about a person
- system: System, architecture, or infrastructure
- rule: Business or technical standard
- project: Project information or description
- epic: Large initiative or feature
- phase: Project phase or checkpoint
- milestone: Important milestone
- task: Individual work item
- step: Sub-step or sub-task

CATEGORY (primary classification domain):
- Architecture: System design, infrastructure, technical decisions
- Security: Security vulnerabilities, threats, safeguards
- Performance: Optimization, speed, efficiency, scalability
- Technical: Code, implementation, debugging, tools
- Business: Strategy, requirements, roadmap, planning
- Operations: Deployment, monitoring, maintenance
- Documentation: Notes, specifications, guides
- Meeting: Discussion, decisions, action items
- Decision: Important choices, trade-offs
- Software Development: Coding, engineering, implementation
- Project Management: Planning, tracking, execution
- Business & Operations: Strategy, team, operations
- Research & Learning: Education, research, knowledge
- Personal Assistant: Personal productivity, life management
- Communication & Collaboration: Messages, feedback, collaboration
- Other: Doesn't fit categories above

CLASSIFICATION (specific within category, optional):
- For Software Development: "Architecture & Design", "Bug & Issue", "Feature Request", "Code Review", "Testing & QA", "Performance", "Security", "Refactoring", "Documentation"
- For Project Management: "Planning & Scope", "Task & Assignment", "Timeline & Deadline", "Resource Allocation", "Risk & Issue", "Progress & Status", "Retrospective"
- For Business: "Strategy & Vision", "Process & Workflow", "Team & People", "Meeting & Discussion", "Decision & Policy", "Vendor & Partnership", "Legal & Compliance"
- Return null or omit if not clearly applicable

PRIORITY (urgency level):
- Critical: Blocks work, security risk, production issue
- High: Important feature, significant bug, needed soon
- Medium: Useful enhancement, minor bug, can wait
- Low: Nice to have, documentation, future consideration

CONTEXT_LABELS (list of 0-3 labels):
- Technical: Contains code, technical details, implementation
- Critical: Important for business or security
- Decision: Contains important decision or choice
- Research: Investigation, testing, exploration
- External: References external systems, people, or tools
- Actionable: Contains action items or next steps
- Pattern: Describes a pattern or best practice

REQUIRED JSON STRUCTURE:
{
  "memory_type": "decision|process|concept|event|person|system|rule|project|epic|phase|milestone|task|step",
  "category": "category name",
  "classification": "specific classification or null",
  "subcategory": "string or null",
  "priority": "Critical|High|Medium|Low",
  "context_labels": ["label1", "label2"],
  "tags": ["tag1", "tag2", ...],
  "confidence": 0.0-1.0
}

Content to classify:
%s

Return ONLY JSON object (start with { end with }), nothing else:
{"memory_type":"decision","category":"Architecture","classification":"Architecture & Design","subcategory":null,"priority":"High","context_labels":["Technical","Actionable"],"tags":["design","system"],"confidence":0.85}`, content)
}

// EntityExtractionPromptWithSettings generates an entity extraction prompt using all available
// entity types from connection settings (system defaults + custom types).
// Falls back to EntityExtractionPrompt if settings is nil.
func EntityExtractionPromptWithSettings(content string, settings *types.SettingsResponse) string {
	if settings == nil || len(settings.AllEntityTypes) == 0 {
		return EntityExtractionPrompt(content)
	}

	// Build type list: system types with descriptions, custom types with name+description
	var typeList strings.Builder
	customTypeMap := make(map[string]types.CustomEntityType)
	for _, ct := range settings.CustomEntityTypes {
		customTypeMap[ct.ID] = ct
	}

	for _, typeID := range settings.AllEntityTypes {
		if ct, isCustom := customTypeMap[typeID]; isCustom {
			desc := ct.Description
			if desc == "" {
				desc = ct.Name
			}
			fmt.Fprintf(&typeList, "- %s: %s (custom)\n", typeID, desc)
		} else if desc, ok := systemEntityTypeDescriptions[typeID]; ok {
			fmt.Fprintf(&typeList, "- %s: %s\n", typeID, desc)
		} else {
			fmt.Fprintf(&typeList, "- %s\n", typeID)
		}
	}

	// Build valid types string for validation line
	validTypes := strings.Join(settings.AllEntityTypes, "|")

	return fmt.Sprintf(`TASK: Extract entities from text.
OUTPUT: ONLY valid JSON. NO markdown. NO code blocks. NO backticks. NO ARRAY - MUST BE OBJECT.

ENTITY TYPES (use ONLY these):
%s
REQUIRED JSON STRUCTURE:
Your response MUST start with { and end with }
Your response MUST have an "entities" key with an array value
Each entity MUST have: name, type, description, confidence

Example structure (EXACT FORMAT REQUIRED):
{
  "entities": [
    {"name":"Alice","type":"person","description":"Works at Google","confidence":0.95},
    {"name":"Google","type":"organization","description":"Tech company","confidence":0.95}
  ]
}

VALIDATION (STRICT):
1. Start with { - End with }
2. "entities" key must be present
3. "entities" value must be an array [...]
4. Each entity is an object with: name, type, description, confidence
5. No extra fields - only these 4 per entity
6. No null values
7. No trailing commas
8. Valid JSON syntax
9. Types EXACTLY: %s
10. Confidence 0.0-1.0

TEXT TO EXTRACT FROM:
%s

RESPOND WITH ONLY THIS JSON STRUCTURE (nothing else):
{"entities":[{"name":"X","type":"person","description":"...","confidence":0.85}]}`, typeList.String(), validTypes, content)
}

// RelationshipExtractionPromptWithSettings generates a relationship extraction prompt using all
// available relationship types from connection settings (system defaults + custom types).
// Falls back to RelationshipExtractionPrompt if settings is nil.
func RelationshipExtractionPromptWithSettings(content string, entities []types.Entity, settings *types.SettingsResponse) string {
	if settings == nil || len(settings.CustomRelationshipTypes) == 0 {
		return RelationshipExtractionPrompt(content, entities)
	}

	// Build entity list
	var entityList strings.Builder
	for i, entity := range entities {
		fmt.Fprintf(&entityList, "- %s (%s)\n", entity.Name, entity.Type)
		if i >= 50 {
			fmt.Fprintf(&entityList, "... and %d more entities\n", len(entities)-50)
			break
		}
	}

	// Separate custom types into bidirectional and unidirectional
	var customBidi, customUni strings.Builder
	for _, ct := range settings.CustomRelationshipTypes {
		if ct.Bidirectional {
			fmt.Fprintf(&customBidi, ", %s", ct.ID)
		} else {
			fmt.Fprintf(&customUni, ", %s", ct.ID)
		}
	}

	return fmt.Sprintf(`Find relationships between these entities. Return ONLY valid JSON, no markdown, no code blocks, no explanation.

BIDIRECTIONAL (use ONE direction, system stores both):
- married_to, colleague_of, works_with, friend_of, knows, sibling_of, partners_with%s

UNIDIRECTIONAL (pick the correct direction):
- employed_by / employs
- manages / managed_by
- reports_to
- leads / led_by
- member_of / has_member
- owns / owned_by
- founded / founded_by
- creates / created_by
- provides / provided_by
- contributes_to
- parent_of / child_of
- contains / belongs_to
- depends_on / required_by
- blocks / blocked_by
- works_on, uses, used_by
- implements, addresses, supersedes, references, documents
- relates_to%s

RULES:
1. Match entity names EXACTLY as listed
2. Confidence: 0.7-0.99
3. If no relationships found, return {"relationships":[]}
4. Use ONLY types from the lists above

Entities (use exact names):
%s

Content to analyze:
%s

Return ONLY JSON object, nothing else, no markdown:
{"relationships":[{"from":"X","to":"Y","type":"...","confidence":0.85},...]}`, customBidi.String(), customUni.String(), entityList.String(), content)
}

// ClassificationExtractionPromptWithSettings generates a classification prompt driven by
// connection settings. If the connection has an active classification category, the prompt
// targets that schema's sub-classifications. Otherwise falls back to the default multi-category prompt.
func ClassificationExtractionPromptWithSettings(content string, settings *types.SettingsResponse) string {
	if settings == nil {
		return ClassificationExtractionPrompt(content)
	}

	// Build memory types list
	allMemoryTypes := settings.AllMemoryTypes
	if len(allMemoryTypes) == 0 {
		allMemoryTypes = types.ValidMemoryTypes
	}
	memoryTypeList := strings.Join(allMemoryTypes, "|")

	// Build memory type descriptions for the prompt
	var memTypeDesc strings.Builder
	for _, mt := range allMemoryTypes {
		fmt.Fprintf(&memTypeDesc, "- %s\n", mt)
	}

	// If active classification category is set, use targeted prompt
	if settings.ActiveClassificationCategory != "" {
		// Find the active schema
		var activeSchema *types.ClassificationSchema
		for i, schema := range settings.AllClassificationSchemas {
			if schema.Category == settings.ActiveClassificationCategory {
				activeSchema = &settings.AllClassificationSchemas[i]
				break
			}
		}
		if activeSchema != nil {
			return buildTargetedClassificationPrompt(content, settings.ActiveClassificationCategory, activeSchema, memoryTypeList, memTypeDesc.String())
		}
	}

	// Fall back to multi-category prompt using all schemas from settings
	if len(settings.AllClassificationSchemas) > 0 {
		return buildMultiCategoryClassificationPrompt(content, settings.AllClassificationSchemas, memoryTypeList, memTypeDesc.String())
	}

	// Final fallback to default hardcoded prompt
	return ClassificationExtractionPrompt(content)
}

// buildTargetedClassificationPrompt creates a classification prompt focused on a single active category.
func buildTargetedClassificationPrompt(content, activeCategory string, schema *types.ClassificationSchema, memoryTypeList, memTypeDesc string) string {
	var classificationList strings.Builder
	for _, clf := range schema.Classifications {
		desc := clf.Description
		if desc == "" {
			desc = clf.Name
		}
		fmt.Fprintf(&classificationList, "- %s: %s\n", clf.Name, desc)
	}

	// Build example classification name
	exampleClf := "General"
	if len(schema.Classifications) > 0 {
		exampleClf = schema.Classifications[0].Name
	}

	return fmt.Sprintf(`TASK: Classify content by memory type, classification, priority, and context.
OUTPUT: ONLY valid JSON. NO markdown. NO code blocks. NO backticks. NO ARRAY - MUST BE OBJECT.

MEMORY TYPE (what kind of memory is this?):
%s
CATEGORY (fixed - always use this exact value):
- %s

CLASSIFICATION (specific type within the category - pick the best match):
%s
PRIORITY (urgency level):
- Critical: Blocks work, security risk, production issue
- High: Important feature, significant bug, needed soon
- Medium: Useful enhancement, minor bug, can wait
- Low: Nice to have, documentation, future consideration

CONTEXT_LABELS (list of 0-3 labels):
- Technical: Contains code, technical details, implementation
- Critical: Important for business or security
- Decision: Contains important decision or choice
- Research: Investigation, testing, exploration
- External: References external systems, people, or tools
- Actionable: Contains action items or next steps
- Pattern: Describes a pattern or best practice

REQUIRED JSON STRUCTURE:
{
  "memory_type": "%s",
  "category": "%s",
  "classification": "classification name from list above",
  "subcategory": null,
  "priority": "Critical|High|Medium|Low",
  "context_labels": ["label1"],
  "tags": ["tag1", "tag2"],
  "confidence": 0.0-1.0
}

Content to classify:
%s

Return ONLY JSON object (start with { end with }), nothing else:
{"memory_type":"concept","category":"%s","classification":"%s","subcategory":null,"priority":"Medium","context_labels":["Technical"],"tags":["example"],"confidence":0.85}`,
		memTypeDesc, activeCategory, classificationList.String(),
		memoryTypeList, activeCategory, content,
		activeCategory, exampleClf)
}

// buildMultiCategoryClassificationPrompt creates a classification prompt using all available schemas.
func buildMultiCategoryClassificationPrompt(content string, schemas []types.ClassificationSchema, memoryTypeList, memTypeDesc string) string {
	var categoryList strings.Builder
	for _, schema := range schemas {
		desc := schema.Description
		if desc == "" {
			desc = schema.Category
		}
		fmt.Fprintf(&categoryList, "- %s: %s\n", schema.Category, desc)
	}

	// Build classification hints per category
	var classificationHints strings.Builder
	for _, schema := range schemas {
		if len(schema.Classifications) == 0 {
			continue
		}
		fmt.Fprintf(&classificationHints, "- For %s: ", schema.Category)
		names := make([]string, 0, len(schema.Classifications))
		for _, clf := range schema.Classifications {
			names = append(names, fmt.Sprintf("%q", clf.Name))
		}
		classificationHints.WriteString(strings.Join(names, ", "))
		classificationHints.WriteString("\n")
	}

	return fmt.Sprintf(`TASK: Classify content by memory type, category, priority, and context.
OUTPUT: ONLY valid JSON. NO markdown. NO code blocks. NO backticks. NO ARRAY - MUST BE OBJECT.

MEMORY TYPE (what kind of memory is this?):
%s
CATEGORY (primary classification domain):
%s
CLASSIFICATION (specific within category, optional):
%s- Return null if not clearly applicable

PRIORITY (urgency level):
- Critical: Blocks work, security risk, production issue
- High: Important feature, significant bug, needed soon
- Medium: Useful enhancement, minor bug, can wait
- Low: Nice to have, documentation, future consideration

CONTEXT_LABELS (list of 0-3 labels):
- Technical, Critical, Decision, Research, External, Actionable, Pattern

REQUIRED JSON STRUCTURE:
{
  "memory_type": "%s",
  "category": "category name",
  "classification": "specific classification or null",
  "subcategory": null,
  "priority": "Critical|High|Medium|Low",
  "context_labels": ["label1"],
  "tags": ["tag1", "tag2"],
  "confidence": 0.0-1.0
}

Content to classify:
%s

Return ONLY JSON object (start with { end with }), nothing else:
{"memory_type":"concept","category":"Other","classification":null,"subcategory":null,"priority":"Medium","context_labels":["Technical"],"tags":[],"confidence":0.85}`,
		memTypeDesc, categoryList.String(), classificationHints.String(),
		memoryTypeList, content)
}
