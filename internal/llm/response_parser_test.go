package llm

import (
	"testing"

	"github.com/scrypster/memento/pkg/types"
)

// ============================================================================
// Helper function for testing extractJSON
// ============================================================================

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantJSON string
	}{
		{
			name:     "plain JSON object",
			input:    `{"key": "value"}`,
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "JSON with markdown code block",
			input:    "```json\n{\"key\": \"value\"}\n```",
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "JSON with triple backticks",
			input:    "```\n{\"key\": \"value\"}\n```",
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "JSON with surrounding text",
			input:    "Here is the JSON:\n{\"key\": \"value\"}\nEnd of JSON",
			wantJSON: `{"key": "value"}`,
		},
		{
			name:     "nested JSON object",
			input:    `{"outer": {"inner": "value"}}`,
			wantJSON: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "JSON with escaped quotes in string",
			input:    `{"text": "He said \"hello\""}`,
			wantJSON: `{"text": "He said \"hello\""}`,
		},
		{
			name:     "JSON with backslash escapes",
			input:    `{"path": "C:\\Users\\test"}`,
			wantJSON: `{"path": "C:\\Users\\test"}`,
		},
		{
			name:     "no JSON present",
			input:    "just some text without json",
			wantJSON: "just some text without json",
		},
		{
			name:     "empty string",
			input:    "",
			wantJSON: "",
		},
		{
			name:     "JSON with newlines in strings",
			input:    `{"text": "line1\nline2"}`,
			wantJSON: `{"text": "line1\nline2"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.wantJSON {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.wantJSON)
			}
		})
	}
}

// ============================================================================
// Tests for ParseEntityResponse
// ============================================================================

func TestParseEntityResponse(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr     string
		wantCount   int
		wantErr     bool
		wantEntities []EntityResponse
	}{
		{
			name:      "valid entities",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "confidence": 0.95}]}`,
			wantCount: 1,
			wantErr:   false,
			wantEntities: []EntityResponse{
				{Name: "John", Type: "person", Confidence: 0.95},
			},
		},
		{
			name:      "multiple valid entities",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "confidence": 0.9}, {"name": "Google", "type": "organization", "confidence": 0.85}]}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty entities array",
			jsonStr:   `{"entities": []}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "malformed JSON",
			jsonStr: `{"entities": [{"name": "John"`,
			wantErr: true,
		},
		{
			name:      "invalid entity type skipped",
			jsonStr:   `{"entities": [{"name": "John", "type": "invalid_type", "confidence": 0.9}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "confidence too high skipped",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "confidence": 1.5}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "confidence negative skipped",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "confidence": -0.1}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "mixed valid and invalid entities",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "confidence": 0.9}, {"name": "BadEnt", "type": "invalid", "confidence": 0.8}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "entity with description field",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "description": "A person", "confidence": 0.9}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "JSON with markdown code block",
			jsonStr:   "```json\n{\"entities\": [{\"name\": \"John\", \"type\": \"person\", \"confidence\": 0.9}]}\n```",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "special characters in entity name",
			jsonStr:   `{"entities": [{"name": "John \"The Boss\" Doe", "type": "person", "confidence": 0.9}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "unicode in entity name",
			jsonStr:   `{"entities": [{"name": "José García", "type": "person", "confidence": 0.9}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "confidence at boundaries",
			jsonStr:   `{"entities": [{"name": "E1", "type": "person", "confidence": 0.0}, {"name": "E2", "type": "person", "confidence": 1.0}]}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "wrong structure key",
			jsonStr:   `{"wrong_key": [{"name": "John", "type": "person", "confidence": 0.9}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "extra fields in entity",
			jsonStr:   `{"entities": [{"name": "John", "type": "person", "confidence": 0.9, "extra_field": "ignored"}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:    "empty string input",
			jsonStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEntityResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntityResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("ParseEntityResponse() got %d entities, want %d", len(got), tt.wantCount)
			}
			if tt.wantEntities != nil && len(got) > 0 {
				for i, entity := range tt.wantEntities {
					if i < len(got) {
						if got[i].Name != entity.Name || got[i].Type != entity.Type || got[i].Confidence != entity.Confidence {
							t.Errorf("ParseEntityResponse() entity %d = %+v, want %+v", i, got[i], entity)
						}
					}
				}
			}
		})
	}
}

// ============================================================================
// Tests for ParseEntityResponseDetailed
// ============================================================================

func TestParseEntityResponseDetailed(t *testing.T) {
	tests := []struct {
		name         string
		jsonStr      string
		wantCount    int
		wantSkipped  int
		wantErr      bool
		checkSkipped func([]SkippedTypeInfo) bool
	}{
		{
			name:        "valid entities with no skipped",
			jsonStr:     `{"entities": [{"name": "John", "type": "person", "confidence": 0.95}]}`,
			wantCount:   1,
			wantSkipped: 0,
			wantErr:     false,
		},
		{
			name:        "invalid type is skipped",
			jsonStr:     `{"entities": [{"name": "BadEnt", "type": "invalid_type", "confidence": 0.9}]}`,
			wantCount:   0,
			wantSkipped: 1,
			wantErr:     false,
			checkSkipped: func(skipped []SkippedTypeInfo) bool {
				return len(skipped) == 1 && skipped[0].Domain == "entity" && skipped[0].TypeName == "invalid_type"
			},
		},
		{
			name:        "mixed valid and skipped",
			jsonStr:     `{"entities": [{"name": "John", "type": "person", "confidence": 0.9}, {"name": "BadEnt", "type": "invalid", "confidence": 0.8}]}`,
			wantCount:   1,
			wantSkipped: 1,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, skipped, err := ParseEntityResponseDetailed(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntityResponseDetailed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(valid) != tt.wantCount {
					t.Errorf("ParseEntityResponseDetailed() got %d valid, want %d", len(valid), tt.wantCount)
				}
				if len(skipped) != tt.wantSkipped {
					t.Errorf("ParseEntityResponseDetailed() got %d skipped, want %d", len(skipped), tt.wantSkipped)
				}
				if tt.checkSkipped != nil && !tt.checkSkipped(skipped) {
					t.Errorf("ParseEntityResponseDetailed() skipped check failed: %+v", skipped)
				}
			}
		})
	}
}

// ============================================================================
// Tests for ParseRelationshipResponse
// ============================================================================

func TestParseRelationshipResponse(t *testing.T) {
	tests := []struct {
		name          string
		jsonStr       string
		wantCount     int
		wantErr       bool
		wantRelations []RelationshipResponse
	}{
		{
			name:      "valid relationship",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.95}]}`,
			wantCount: 1,
			wantErr:   false,
			wantRelations: []RelationshipResponse{
				{From: "John", To: "Google", Type: "employed_by", Confidence: 0.95},
			},
		},
		{
			name:      "multiple valid relationships",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.9}, {"from": "Jane", "to": "Facebook", "type": "employed_by", "confidence": 0.85}]}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "empty relationships array",
			jsonStr:   `{"relationships": []}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "malformed JSON",
			jsonStr: `{"relationships": [{"from": "John"`,
			wantErr: true,
		},
		{
			name:      "invalid relationship type skipped",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "invalid_type", "confidence": 0.9}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "confidence too high skipped",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 1.5}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "confidence negative skipped",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": -0.1}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "mixed valid and invalid relationships",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.9}, {"from": "BadRel", "to": "Entity", "type": "invalid", "confidence": 0.8}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "JSON with markdown code block",
			jsonStr:   "```json\n{\"relationships\": [{\"from\": \"John\", \"to\": \"Google\", \"type\": \"employed_by\", \"confidence\": 0.9}]}\n```",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "special characters in from/to",
			jsonStr:   `{"relationships": [{"from": "John \"The Boss\" Doe", "to": "Google Inc.", "type": "employed_by", "confidence": 0.9}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "confidence at boundaries",
			jsonStr:   `{"relationships": [{"from": "A", "to": "B", "type": "employed_by", "confidence": 0.0}, {"from": "C", "to": "D", "type": "employed_by", "confidence": 1.0}]}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "wrong structure key",
			jsonStr:   `{"wrong_key": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.9}]}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "extra fields in relationship",
			jsonStr:   `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.9, "extra_field": "ignored"}]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:    "empty string input",
			jsonStr: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRelationshipResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRelationshipResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("ParseRelationshipResponse() got %d relationships, want %d", len(got), tt.wantCount)
			}
			if tt.wantRelations != nil && len(got) > 0 {
				for i, rel := range tt.wantRelations {
					if i < len(got) {
						if got[i].From != rel.From || got[i].To != rel.To || got[i].Type != rel.Type || got[i].Confidence != rel.Confidence {
							t.Errorf("ParseRelationshipResponse() relationship %d = %+v, want %+v", i, got[i], rel)
						}
					}
				}
			}
		})
	}
}

// ============================================================================
// Tests for ParseRelationshipResponseDetailed
// ============================================================================

func TestParseRelationshipResponseDetailed(t *testing.T) {
	tests := []struct {
		name         string
		jsonStr      string
		wantCount    int
		wantSkipped  int
		wantErr      bool
		checkSkipped func([]SkippedTypeInfo) bool
	}{
		{
			name:        "valid relationships with no skipped",
			jsonStr:     `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.95}]}`,
			wantCount:   1,
			wantSkipped: 0,
			wantErr:     false,
		},
		{
			name:        "invalid type is skipped",
			jsonStr:     `{"relationships": [{"from": "John", "to": "Google", "type": "invalid_type", "confidence": 0.9}]}`,
			wantCount:   0,
			wantSkipped: 1,
			wantErr:     false,
			checkSkipped: func(skipped []SkippedTypeInfo) bool {
				return len(skipped) == 1 && skipped[0].Domain == "relationship" && skipped[0].TypeName == "invalid_type"
			},
		},
		{
			name:        "mixed valid and skipped",
			jsonStr:     `{"relationships": [{"from": "John", "to": "Google", "type": "employed_by", "confidence": 0.9}, {"from": "BadRel", "to": "Entity", "type": "invalid", "confidence": 0.8}]}`,
			wantCount:   1,
			wantSkipped: 1,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, skipped, err := ParseRelationshipResponseDetailed(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRelationshipResponseDetailed() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(valid) != tt.wantCount {
					t.Errorf("ParseRelationshipResponseDetailed() got %d valid, want %d", len(valid), tt.wantCount)
				}
				if len(skipped) != tt.wantSkipped {
					t.Errorf("ParseRelationshipResponseDetailed() got %d skipped, want %d", len(skipped), tt.wantSkipped)
				}
				if tt.checkSkipped != nil && !tt.checkSkipped(skipped) {
					t.Errorf("ParseRelationshipResponseDetailed() skipped check failed: %+v", skipped)
				}
			}
		})
	}
}

// ============================================================================
// Tests for ParseSummarizationResponse
// ============================================================================

func TestParseSummarizationResponse(t *testing.T) {
	tests := []struct {
		name         string
		jsonStr      string
		wantErr      bool
		checkResult  func(*SummarizationResponse) bool
	}{
		{
			name:    "valid summarization",
			jsonStr: `{"summary": "This is a summary", "key_points": ["Point 1", "Point 2"]}`,
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return resp.Summary == "This is a summary" && len(resp.KeyPoints) == 2
			},
		},
		{
			name:    "summarization with empty key_points",
			jsonStr: `{"summary": "Summary", "key_points": []}`,
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return resp.Summary == "Summary" && len(resp.KeyPoints) == 0
			},
		},
		{
			name:    "summarization with markdown code block",
			jsonStr: "```json\n{\"summary\": \"Test\", \"key_points\": [\"P1\"]}\n```",
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return resp.Summary == "Test"
			},
		},
		{
			name:    "malformed JSON",
			jsonStr: `{"summary": "Test"`,
			wantErr: true,
		},
		{
			name:    "summary with special characters",
			jsonStr: `{"summary": "Summary with \"quotes\" and \\backslashes", "key_points": []}`,
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return len(resp.Summary) > 0
			},
		},
		{
			name:    "summary with unicode",
			jsonStr: `{"summary": "Résumé with éàü", "key_points": ["Pöint"]}`,
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return len(resp.Summary) > 0 && len(resp.KeyPoints) > 0
			},
		},
		{
			name:    "empty string input",
			jsonStr: "",
			wantErr: true,
		},
		{
			name:    "null key_points",
			jsonStr: `{"summary": "Test", "key_points": null}`,
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return resp.KeyPoints == nil
			},
		},
		{
			name:    "extra fields ignored",
			jsonStr: `{"summary": "Test", "key_points": ["P1"], "extra": "ignored"}`,
			wantErr: false,
			checkResult: func(resp *SummarizationResponse) bool {
				return resp.Summary == "Test"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSummarizationResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSummarizationResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil && !tt.checkResult(got) {
				t.Errorf("ParseSummarizationResponse() result check failed: %+v", got)
			}
		})
	}
}

// ============================================================================
// Tests for ParseKeywordResponse
// ============================================================================

func TestParseKeywordResponse(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr     string
		wantCount   int
		wantErr     bool
		checkResult func([]string) bool
	}{
		{
			name:      "valid keywords",
			jsonStr:   `{"keywords": ["keyword1", "keyword2", "keyword3"]}`,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "single keyword",
			jsonStr:   `{"keywords": ["solo"]}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "empty keywords array",
			jsonStr:   `{"keywords": []}`,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:    "malformed JSON",
			jsonStr: `{"keywords": ["keyword1"`,
			wantErr: true,
		},
		{
			name:      "keywords with special characters",
			jsonStr:   `{"keywords": ["key \"word\"", "key\\word"]}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "keywords with unicode",
			jsonStr:   `{"keywords": ["Résumé", "café"]}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:    "empty string input",
			jsonStr: "",
			wantErr: true,
		},
		{
			name:      "null keywords",
			jsonStr:   `{"keywords": null}`,
			wantCount: 0,
			wantErr:   false,
			checkResult: func(kw []string) bool {
				return kw == nil
			},
		},
		{
			name:      "extra fields ignored",
			jsonStr:   `{"keywords": ["k1", "k2"], "extra": "ignored"}`,
			wantCount: 2,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseKeywordResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseKeywordResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("ParseKeywordResponse() got %d keywords, want %d", len(got), tt.wantCount)
			}
			if tt.checkResult != nil && !tt.checkResult(got) {
				t.Errorf("ParseKeywordResponse() result check failed: %+v", got)
			}
		})
	}
}

// ============================================================================
// Tests for ParseClassificationResponse
// ============================================================================

func TestParseClassificationResponse(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr     string
		wantErr     bool
		checkResult func(*ClassificationResponse) bool
	}{
		{
			name:    "valid classification",
			jsonStr: `{"memory_type": "decision", "category": "Architecture", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			wantErr: false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.MemoryType == "decision" && resp.Priority == "High" && resp.Confidence == 0.9
			},
		},
		{
			name:    "classification with optional fields",
			jsonStr: `{"memory_type": "concept", "category": "Technical", "classification": "Design Pattern", "priority": "Medium", "context_labels": ["label1"], "tags": ["tag1"], "confidence": 0.85}`,
			wantErr: false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.MemoryType == "concept" && resp.Classification == "Design Pattern"
			},
		},
		{
			name:    "classification with subcategory",
			jsonStr: `{"memory_type": "process", "category": "Business", "priority": "Critical", "context_labels": [], "tags": [], "confidence": 0.95, "subcategory": "Workflow"}`,
			wantErr: false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.MemoryType == "process" && resp.Subcategory != nil && *resp.Subcategory == "Workflow"
			},
		},
		{
			name:    "invalid memory type",
			jsonStr: `{"memory_type": "invalid_type", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			wantErr: true,
		},
		{
			name:    "invalid priority",
			jsonStr: `{"memory_type": "decision", "category": "Technical", "priority": "InvalidPriority", "context_labels": [], "tags": [], "confidence": 0.9}`,
			wantErr: true,
		},
		{
			name:    "confidence out of range (too high)",
			jsonStr: `{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 1.5}`,
			wantErr: true,
		},
		{
			name:    "confidence out of range (negative)",
			jsonStr: `{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": -0.1}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			jsonStr: `{"memory_type": "decision"`,
			wantErr: true,
		},
		{
			name:    "JSON with markdown code block",
			jsonStr: "```json\n{\"memory_type\": \"decision\", \"category\": \"Technical\", \"priority\": \"High\", \"context_labels\": [], \"tags\": [], \"confidence\": 0.9}\n```",
			wantErr: false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.MemoryType == "decision"
			},
		},
		{
			name:    "confidence at boundaries",
			jsonStr: `{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.0}`,
			wantErr: false,
		},
		{
			name:    "confidence at upper boundary",
			jsonStr: `{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 1.0}`,
			wantErr: false,
		},
		{
			name:    "empty string input",
			jsonStr: "",
			wantErr: true,
		},
		{
			name:    "all valid memory types - decision",
			jsonStr: `{"memory_type": "decision", "category": "Other", "priority": "Low", "context_labels": [], "tags": [], "confidence": 0.5}`,
			wantErr: false,
		},
		{
			name:    "all valid memory types - person",
			jsonStr: `{"memory_type": "person", "category": "Other", "priority": "Low", "context_labels": [], "tags": [], "confidence": 0.5}`,
			wantErr: false,
		},
		{
			name:    "all valid priorities",
			jsonStr: `{"memory_type": "decision", "category": "Other", "priority": "Critical", "context_labels": [], "tags": [], "confidence": 0.5}`,
			wantErr: false,
		},
		{
			name:    "custom category allowed",
			jsonStr: `{"memory_type": "decision", "category": "CustomCategory", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			wantErr: false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.Category == "CustomCategory"
			},
		},
		{
			name:    "special characters in category",
			jsonStr: `{"memory_type": "decision", "category": "Tech & Security", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseClassificationResponse(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClassificationResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil && !tt.checkResult(got) {
				t.Errorf("ParseClassificationResponse() result check failed: %+v", got)
			}
		})
	}
}

// ============================================================================
// Tests for ValidateEntityTypes
// ============================================================================

func TestValidateEntityTypes(t *testing.T) {
	tests := []struct {
		name    string
		entities []EntityResponse
		wantErr bool
	}{
		{
			name:    "all valid entity types",
			entities: []EntityResponse{{Name: "E1", Type: "person"}, {Name: "E2", Type: "organization"}},
			wantErr: false,
		},
		{
			name:    "single valid entity",
			entities: []EntityResponse{{Name: "E1", Type: "person"}},
			wantErr: false,
		},
		{
			name:    "invalid entity type",
			entities: []EntityResponse{{Name: "E1", Type: "invalid_type"}},
			wantErr: true,
		},
		{
			name:    "mixed valid and invalid",
			entities: []EntityResponse{{Name: "E1", Type: "person"}, {Name: "E2", Type: "invalid"}},
			wantErr: true,
		},
		{
			name:    "empty slice",
			entities: []EntityResponse{},
			wantErr: false,
		},
		{
			name:    "all 22 valid entity types",
			entities: []EntityResponse{
				{Name: "E", Type: "person"},
				{Name: "E", Type: "organization"},
				{Name: "E", Type: "project"},
				{Name: "E", Type: "location"},
				{Name: "E", Type: "event"},
				{Name: "E", Type: "document"},
				{Name: "E", Type: "note"},
				{Name: "E", Type: "file"},
				{Name: "E", Type: "url"},
				{Name: "E", Type: "email"},
				{Name: "E", Type: "message"},
				{Name: "E", Type: "concept"},
				{Name: "E", Type: "task"},
				{Name: "E", Type: "repository"},
				{Name: "E", Type: "code_snippet"},
				{Name: "E", Type: "api"},
				{Name: "E", Type: "database"},
				{Name: "E", Type: "server"},
				{Name: "E", Type: "tool"},
				{Name: "E", Type: "framework"},
				{Name: "E", Type: "language"},
				{Name: "E", Type: "library"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEntityTypes(tt.entities)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEntityTypes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// Tests for ValidateRelationshipTypes
// ============================================================================

func TestValidateRelationshipTypes(t *testing.T) {
	tests := []struct {
		name     string
		rels     []RelationshipResponse
		wantCount int
	}{
		{
			name:      "all valid relationships",
			rels:      []RelationshipResponse{{From: "A", To: "B", Type: "employed_by"}, {From: "C", To: "D", Type: "knows"}},
			wantCount: 2,
		},
		{
			name:      "single valid relationship",
			rels:      []RelationshipResponse{{From: "A", To: "B", Type: "employed_by"}},
			wantCount: 1,
		},
		{
			name:      "single invalid relationship",
			rels:      []RelationshipResponse{{From: "A", To: "B", Type: "invalid_type"}},
			wantCount: 0,
		},
		{
			name:      "mixed valid and invalid",
			rels:      []RelationshipResponse{{From: "A", To: "B", Type: "employed_by"}, {From: "C", To: "D", Type: "invalid"}},
			wantCount: 1,
		},
		{
			name:      "empty slice",
			rels:      []RelationshipResponse{},
			wantCount: 0,
		},
		{
			name: "multiple invalid types",
			rels: []RelationshipResponse{
				{From: "A", To: "B", Type: "invalid1"},
				{From: "C", To: "D", Type: "invalid2"},
				{From: "E", To: "F", Type: "employed_by"},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateRelationshipTypes(tt.rels)
			if len(got) != tt.wantCount {
				t.Errorf("ValidateRelationshipTypes() got %d, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// ============================================================================
// Tests for ParseEntityResponseWithTypes
// ============================================================================

func TestParseEntityResponseWithTypes(t *testing.T) {
	tests := []struct {
		name         string
		jsonStr      string
		allowedTypes []string
		wantCount    int
		wantErr      bool
	}{
		{
			name:         "entity with allowed type",
			jsonStr:      `{"entities": [{"name": "E1", "type": "person", "confidence": 0.9}]}`,
			allowedTypes: []string{"person", "organization"},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "entity with disallowed type",
			jsonStr:      `{"entities": [{"name": "E1", "type": "person", "confidence": 0.9}]}`,
			allowedTypes: []string{"organization", "project"},
			wantCount:    0,
			wantErr:      false,
		},
		{
			name:         "mixed allowed and disallowed",
			jsonStr:      `{"entities": [{"name": "E1", "type": "person", "confidence": 0.9}, {"name": "E2", "type": "organization", "confidence": 0.8}]}`,
			allowedTypes: []string{"person"},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "empty allowed types falls back to default",
			jsonStr:      `{"entities": [{"name": "E1", "type": "person", "confidence": 0.9}]}`,
			allowedTypes: []string{},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "custom entity type allowed",
			jsonStr:      `{"entities": [{"name": "E1", "type": "custom_type", "confidence": 0.9}]}`,
			allowedTypes: []string{"custom_type"},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "malformed JSON",
			jsonStr:      `{"entities": [{"name": "E1"`,
			allowedTypes: []string{"person"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseEntityResponseWithTypes(tt.jsonStr, tt.allowedTypes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEntityResponseWithTypes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("ParseEntityResponseWithTypes() got %d, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// ============================================================================
// Tests for ParseRelationshipResponseWithTypes
// ============================================================================

func TestParseRelationshipResponseWithTypes(t *testing.T) {
	tests := []struct {
		name         string
		jsonStr      string
		allowedTypes []string
		wantCount    int
		wantErr      bool
	}{
		{
			name:         "relationship with allowed type",
			jsonStr:      `{"relationships": [{"from": "A", "to": "B", "type": "employed_by", "confidence": 0.9}]}`,
			allowedTypes: []string{"employed_by", "knows"},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "relationship with disallowed type",
			jsonStr:      `{"relationships": [{"from": "A", "to": "B", "type": "employed_by", "confidence": 0.9}]}`,
			allowedTypes: []string{"knows", "friend_of"},
			wantCount:    0,
			wantErr:      false,
		},
		{
			name:         "mixed allowed and disallowed",
			jsonStr:      `{"relationships": [{"from": "A", "to": "B", "type": "employed_by", "confidence": 0.9}, {"from": "C", "to": "D", "type": "knows", "confidence": 0.8}]}`,
			allowedTypes: []string{"employed_by"},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "empty allowed types falls back to default",
			jsonStr:      `{"relationships": [{"from": "A", "to": "B", "type": "employed_by", "confidence": 0.9}]}`,
			allowedTypes: []string{},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "custom relationship type allowed",
			jsonStr:      `{"relationships": [{"from": "A", "to": "B", "type": "custom_type", "confidence": 0.9}]}`,
			allowedTypes: []string{"custom_type"},
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "malformed JSON",
			jsonStr:      `{"relationships": [{"from": "A"`,
			allowedTypes: []string{"employed_by"},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRelationshipResponseWithTypes(tt.jsonStr, tt.allowedTypes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRelationshipResponseWithTypes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("ParseRelationshipResponseWithTypes() got %d, want %d", len(got), tt.wantCount)
			}
		})
	}
}

// ============================================================================
// Tests for ParseClassificationResponseWithSettings
// ============================================================================

func TestParseClassificationResponseWithSettings(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr     string
		settings    *types.SettingsResponse
		wantErr     bool
		checkResult func(*ClassificationResponse) bool
	}{
		{
			name:     "nil settings falls back to default validation",
			jsonStr:  `{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			settings: nil,
			wantErr:  false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.MemoryType == "decision"
			},
		},
		{
			name:    "valid memory type with custom settings",
			jsonStr: `{"memory_type": "custom_type", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			settings: &types.SettingsResponse{
				AllMemoryTypes: []string{"custom_type", "another_type"},
			},
			wantErr: false,
			checkResult: func(resp *ClassificationResponse) bool {
				return resp.MemoryType == "custom_type"
			},
		},
		{
			name:    "invalid memory type with custom settings",
			jsonStr: `{"memory_type": "invalid_type", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			settings: &types.SettingsResponse{
				AllMemoryTypes: []string{"custom_type", "another_type"},
			},
			wantErr: true,
		},
		{
			name:    "invalid priority with settings",
			jsonStr: `{"memory_type": "custom_type", "category": "Technical", "priority": "InvalidPriority", "context_labels": [], "tags": [], "confidence": 0.9}`,
			settings: &types.SettingsResponse{
				AllMemoryTypes: []string{"custom_type"},
			},
			wantErr: true,
		},
		{
			name:    "invalid confidence with settings",
			jsonStr: `{"memory_type": "custom_type", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 1.5}`,
			settings: &types.SettingsResponse{
				AllMemoryTypes: []string{"custom_type"},
			},
			wantErr: true,
		},
		{
			name:    "empty AllMemoryTypes in settings uses default validation",
			jsonStr: `{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`,
			settings: &types.SettingsResponse{
				AllMemoryTypes: []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseClassificationResponseWithSettings(tt.jsonStr, tt.settings)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseClassificationResponseWithSettings() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkResult != nil && !tt.checkResult(got) {
				t.Errorf("ParseClassificationResponseWithSettings() result check failed: %+v", got)
			}
		})
	}
}

// ============================================================================
// Edge cases and integration tests
// ============================================================================

func TestParseEntityResponseMultipleBatches(t *testing.T) {
	// Test multiple entity parsing calls in sequence
	tests := []string{
		`{"entities": [{"name": "Alice", "type": "person", "confidence": 0.95}]}`,
		`{"entities": [{"name": "Bob", "type": "person", "confidence": 0.85}]}`,
		`{"entities": [{"name": "TechCorp", "type": "organization", "confidence": 0.9}]}`,
	}

	for i, jsonStr := range tests {
		entities, err := ParseEntityResponse(jsonStr)
		if err != nil {
			t.Errorf("batch %d: ParseEntityResponse() error = %v", i, err)
		}
		if len(entities) != 1 {
			t.Errorf("batch %d: got %d entities, want 1", i, len(entities))
		}
	}
}

func TestParseResponsesWithLargeConfidenceValues(t *testing.T) {
	// Test boundary conditions for confidence values
	testCases := map[string]bool{
		`{"entities": [{"name": "E", "type": "person", "confidence": 0.0}]}`:      true,
		`{"entities": [{"name": "E", "type": "person", "confidence": 0.5}]}`:      true,
		`{"entities": [{"name": "E", "type": "person", "confidence": 1.0}]}`:      true,
		`{"entities": [{"name": "E", "type": "person", "confidence": -0.001}]}`:   false,
		`{"entities": [{"name": "E", "type": "person", "confidence": 1.001}]}`:    false,
		`{"entities": [{"name": "E", "type": "person", "confidence": 999.0}]}`:    false,
		`{"entities": [{"name": "E", "type": "person", "confidence": -999.0}]}`:   false,
	}

	for jsonStr, shouldPass := range testCases {
		entities, err := ParseEntityResponse(jsonStr)
		if err != nil {
			if shouldPass {
				t.Errorf("ParseEntityResponse(%s) unexpected error: %v", jsonStr, err)
			}
			continue
		}
		if shouldPass && len(entities) != 1 {
			t.Errorf("ParseEntityResponse(%s) expected 1 entity, got %d", jsonStr, len(entities))
		}
		if !shouldPass && len(entities) != 0 {
			t.Errorf("ParseEntityResponse(%s) expected 0 entities, got %d", jsonStr, len(entities))
		}
	}
}

func TestParseResponsesWithComplexJSON(t *testing.T) {
	// Test with deeply nested JSON structures with extra text
	complexJSON := "Here's the entity extraction result:\n\n```json\n{\n  \"entities\": [\n    {\n      \"name\": \"John Doe\",\n      \"type\": \"person\",\n      \"description\": \"A software engineer with expertise in Go and distributed systems\",\n      \"confidence\": 0.98\n    },\n    {\n      \"name\": \"Google\",\n      \"type\": \"organization\",\n      \"confidence\": 0.99\n    }\n  ]\n}\n```\n\nThis extraction was completed successfully."

	entities, err := ParseEntityResponse(complexJSON)
	if err != nil {
		t.Fatalf("ParseEntityResponse() error = %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("ParseEntityResponse() got %d entities, want 2", len(entities))
	}
	if entities[0].Name != "John Doe" {
		t.Errorf("ParseEntityResponse() first entity name = %q, want %q", entities[0].Name, "John Doe")
	}
}

func TestParseResponsesWithMixedValidationErrors(t *testing.T) {
	// Test parsing with multiple validation errors to ensure all are handled
	jsonStr := `{
		"entities": [
			{"name": "Valid1", "type": "person", "confidence": 0.9},
			{"name": "BadType", "type": "invalid_type", "confidence": 0.9},
			{"name": "BadConf", "type": "person", "confidence": 1.5},
			{"name": "Valid2", "type": "organization", "confidence": 0.8},
			{"name": "BadConf2", "type": "project", "confidence": -0.1},
			{"name": "BadType2", "type": "unknown", "confidence": 0.7}
		]
	}`

	entities, err := ParseEntityResponse(jsonStr)
	if err != nil {
		t.Fatalf("ParseEntityResponse() error = %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("ParseEntityResponse() got %d entities, want 2", len(entities))
	}
	// Should only have the two valid entries
	if entities[0].Name != "Valid1" || entities[1].Name != "Valid2" {
		t.Errorf("ParseEntityResponse() got unexpected entities")
	}
}
