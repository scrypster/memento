package llm

import (
	"testing"
)

// ============================================================================
// FuzzParseEntityResponse - fuzzes entity extraction JSON parsing
// ============================================================================

func FuzzParseEntityResponse(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": 0.9}]}`)
	f.Add(``)
	f.Add(`{"entities": null}`)
	f.Add(`not json at all`)
	f.Add("```json\n{\"entities\": []}\n```")
	f.Add(`{"entities": []}`)
	f.Add(`{"entities": [{"name": "truncated"`)
	f.Add(`{"entities": [{"name": "", "type": "", "confidence": 0}]}`)
	f.Add(`{"entities": [{"name": "x", "type": "person", "confidence": 0.0}, {"name": "y", "type": "organization", "confidence": 1.0}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": 1.5}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": -0.5}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "invalid_type_xyz", "confidence": 0.9}]}`)
	f.Add(`{"entities": [{"name": "José García", "type": "person", "confidence": 0.95}]}`)
	f.Add(`{"entities": [{"name": "John \"The Boss\" Doe", "type": "person", "confidence": 0.9}]}`)
	f.Add(`{"entities": [{"name": "test\\nwith\\nnewlines", "type": "person", "confidence": 0.9}]}`)
	f.Add(`{"nested": {"entities": [{"name": "test", "type": "person", "confidence": 0.9}]}}`)
	f.Add(`{"entities": [{"name": "a", "type": "person", "confidence": 0.9}, {"name": "b", "type": "invalid", "confidence": 0.8}, {"name": "c", "type": "organization", "confidence": 2.0}]}`)
	f.Add(`{"entities": [{"name": "very_long_name_` + string(make([]byte, 1000)) + `", "type": "person", "confidence": 0.9}]}`)
	f.Add(`{{{`)
	f.Add(`[{"name": "test", "type": "person", "confidence": 0.9}]`)
	f.Add(`{"entities": [{"name": null, "type": "person", "confidence": 0.9}]}`)
	f.Add(`{"entities": [{"name": "test", "type": null, "confidence": 0.9}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": null}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": "0.9"}]}`)
	f.Add("```\n{\"entities\": [{\"name\": \"test\", \"type\": \"person\", \"confidence\": 0.9}]}\n```")
	f.Add(`Text before {"entities": [{"name": "test", "type": "person", "confidence": 0.9}]} text after`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": 0.9, "extra": "field", "another": 123}]}`)

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseEntityResponse panicked on input %q: %v", input, r)
			}
		}()
		_, _ = ParseEntityResponse(input)
	})
}

// ============================================================================
// FuzzParseRelationshipResponse - fuzzes relationship extraction JSON parsing
// ============================================================================

func FuzzParseRelationshipResponse(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(``)
	f.Add(`{"relationships": null}`)
	f.Add(`not json at all`)
	f.Add("```json\n{\"relationships\": []}\n```")
	f.Add(`{"relationships": []}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b"`)
	f.Add(`{"relationships": [{"from": "", "to": "", "type": "", "confidence": 0}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.0}, {"from": "c", "to": "d", "type": "knows", "confidence": 1.0}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 1.5}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": -0.5}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "invalid_type_xyz", "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "José García", "to": "Francisco López", "type": "employed_by", "confidence": 0.95}]}`)
	f.Add(`{"relationships": [{"from": "John \"The Boss\" Doe", "to": "Google Inc.", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "a/b/c", "to": "x\\y\\z", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "a\nb", "to": "c\nd", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(`{"nested": {"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}]}}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}, {"from": "c", "to": "d", "type": "invalid", "confidence": 0.8}, {"from": "e", "to": "f", "type": "knows", "confidence": 2.0}]}`)
	f.Add(`{"relationships": [{"from": "` + string(make([]byte, 1000)) + `", "to": "b", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(`{{{`)
	f.Add(`[{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}]`)
	f.Add(`{"relationships": [{"from": null, "to": "b", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": null, "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": null, "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": null}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": "0.9"}]}`)
	f.Add("```\n{\"relationships\": [{\"from\": \"a\", \"to\": \"b\", \"type\": \"employed_by\", \"confidence\": 0.9}]}\n```")
	f.Add(`Text before {"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}]} text after`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9, "extra": "field"}]}`)

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseRelationshipResponse panicked on input %q: %v", input, r)
			}
		}()
		_, _ = ParseRelationshipResponse(input)
	})
}

// ============================================================================
// FuzzParseClassificationResponse - fuzzes classification JSON parsing
// ============================================================================

func FuzzParseClassificationResponse(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(``)
	f.Add(`not json at all`)
	f.Add(`{"memory_type": "invalid_type", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "InvalidPriority", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 1.5}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": -0.1}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.0}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 1.0}`)
	f.Add(`{"memory_type": "person", "category": "Other", "priority": "Critical", "context_labels": [], "tags": [], "confidence": 0.5}`)
	f.Add(`{"memory_type": "process", "category": "Business", "priority": "Medium", "context_labels": [], "tags": [], "confidence": 0.7}`)
	f.Add(`{"memory_type": "event", "category": "Meeting", "priority": "Low", "context_labels": ["label1"], "tags": ["tag1"], "confidence": 0.8}`)
	f.Add(`{"memory_type": "decision", "category": "Custom Category", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add("```json\n{\"memory_type\": \"decision\", \"category\": \"Technical\", \"priority\": \"High\", \"context_labels\": [], \"tags\": [], \"confidence\": 0.9}\n```")
	f.Add(`Text before {"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9} text after`)
	f.Add(`{"memory_type": "decision", "category": "Tech & Security", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": null, "tags": null, "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "classification": "SubType", "subcategory": "Category", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": null, "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": null, "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": null, "context_labels": [], "tags": [], "confidence": 0.9}`)
	f.Add(`{"memory_type": "decision", "category": "Technical", "priority": "High", "context_labels": [], "tags": [], "confidence": null}`)
	f.Add(`{{{`)
	f.Add(`{"memory_type": "decision"`)
	f.Add(`{"memory_type": "decision", "category": "Very Long Category Name With Many Words And Special Characters Like !@#$%^&*()", "priority": "High", "context_labels": [], "tags": [], "confidence": 0.9}`)

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseClassificationResponse panicked on input %q: %v", input, r)
			}
		}()
		_, _ = ParseClassificationResponse(input)
	})
}

// ============================================================================
// FuzzParseSummarizationResponse - fuzzes summarization JSON parsing
// ============================================================================

func FuzzParseSummarizationResponse(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"summary": "This is a summary", "key_points": ["Point 1", "Point 2"]}`)
	f.Add(``)
	f.Add(`not json at all`)
	f.Add(`{"summary": "", "key_points": []}`)
	f.Add(`{"summary": "Summary", "key_points": null}`)
	f.Add("```json\n{\"summary\": \"Test\", \"key_points\": [\"P1\"]}\n```")
	f.Add(`Text before {"summary": "Summary", "key_points": []} text after`)
	f.Add(`{"summary": "Summary with \"quotes\" and \\backslashes", "key_points": []}`)
	f.Add(`{"summary": "Résumé with éàü", "key_points": ["Pöint"]}`)
	f.Add(`{"summary": "Multi\nline\nsummary", "key_points": ["P1", "P2"]}`)
	f.Add(`{"summary": "` + string(make([]byte, 5000)) + `", "key_points": []}`)
	f.Add(`{"summary": "Summary", "key_points": ["` + string(make([]byte, 1000)) + `"]}`)
	f.Add(`{"summary": null, "key_points": []}`)
	f.Add(`{"summary": "Summary", "key_points": "not_an_array"}`)
	f.Add(`{{{`)
	f.Add(`{"summary": "Test"`)
	f.Add(`{"summary": "Summary", "key_points": [null, "valid", null]}`)
	f.Add(`{"summary": "Summary", "key_points": [], "extra": "field", "another": 123}`)

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseSummarizationResponse panicked on input %q: %v", input, r)
			}
		}()
		_, _ = ParseSummarizationResponse(input)
	})
}

// ============================================================================
// FuzzParseKeywordResponse - fuzzes keyword extraction JSON parsing
// ============================================================================

func FuzzParseKeywordResponse(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"keywords": ["keyword1", "keyword2", "keyword3"]}`)
	f.Add(``)
	f.Add(`not json at all`)
	f.Add(`{"keywords": []}`)
	f.Add(`{"keywords": null}`)
	f.Add(`{"keywords": ["solo"]}`)
	f.Add(`{"keywords": ["key \"word\"", "key\\word"]}`)
	f.Add(`{"keywords": ["Résumé", "café"]}`)
	f.Add(`{"keywords": ["` + string(make([]byte, 1000)) + `", "short"]}`)
	f.Add(`{"keywords": ["k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9", "k10"]}`)
	f.Add("```json\n{\"keywords\": [\"k1\", \"k2\"]}\n```")
	f.Add(`Text before {"keywords": ["k1", "k2"]} text after`)
	f.Add(`{"keywords": null}`)
	f.Add(`{"keywords": "not_an_array"}`)
	f.Add(`{"keywords": [null, "valid", null]}`)
	f.Add(`{{{`)
	f.Add(`{"keywords": ["k1"`)
	f.Add(`{"keywords": ["k1", "k2"], "extra": "field"}`)

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseKeywordResponse panicked on input %q: %v", input, r)
			}
		}()
		_, _ = ParseKeywordResponse(input)
	})
}

// ============================================================================
// FuzzExtractJSON - fuzzes the JSON extraction helper function
// ============================================================================

func FuzzExtractJSON(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"key": "value"}`)
	f.Add(``)
	f.Add(`just plain text`)
	f.Add("```json\n{\"key\": \"value\"}\n```")
	f.Add("```\n{\"key\": \"value\"}\n```")
	f.Add(`Text before {"key": "value"} text after`)
	f.Add(`{"outer": {"inner": "value"}}`)
	f.Add(`{"text": "He said \"hello\""}`)
	f.Add(`{"path": "C:\\Users\\test"}`)
	f.Add(`{"text": "line1\nline2"}`)
	f.Add(`{`)
	f.Add(`}`)
	f.Add(`{{{`)
	f.Add(`}}}`)
	f.Add(`{"key": "value"}{"another": "object"}`)
	f.Add("Text with ``` triple backticks but no content")
	f.Add("```json\nincomplete json")
	f.Add(`{"escaped": "\\\"quote\\\""}`)
	f.Add(`{"unicode": "😀🎉🔥"}`)
	f.Add(`Multiple {"objects": 1} and {"more": 2}`)
	f.Add(`{"": ""}`)
	f.Add(`{"key": {"nested": {"deeply": {"object": "value"}}}}`)
	f.Add(`{"key": "value with\nmultiple\nlines"}`)
	f.Add(string(make([]byte, 10000)))
	f.Add(`{"a":1,"b":2,"c":3,"d":4,"e":5}`)

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("extractJSON panicked on input %q: %v", input, r)
			}
		}()
		_ = extractJSON(input)
	})
}

// ============================================================================
// FuzzParseEntityResponseDetailed - fuzzes detailed entity parsing
// ============================================================================

func FuzzParseEntityResponseDetailed(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": 0.9}]}`)
	f.Add(``)
	f.Add(`{"entities": []}`)
	f.Add(`not json at all`)
	f.Add(`{"entities": [{"name": "test", "type": "invalid_type", "confidence": 0.9}]}`)
	f.Add(`{"entities": [{"name": "valid", "type": "person", "confidence": 0.9}, {"name": "invalid", "type": "unknown", "confidence": 0.8}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": 1.5}]}`)
	f.Add(`{"entities": [{"name": "test", "type": "person", "confidence": -0.1}]}`)
	f.Add(`{{{`)
	f.Add("```json\n{\"entities\": [{\"name\": \"test\", \"type\": \"person\", \"confidence\": 0.9}]}\n```")

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseEntityResponseDetailed panicked on input %q: %v", input, r)
			}
		}()
		_, _, _ = ParseEntityResponseDetailed(input)
	})
}

// ============================================================================
// FuzzParseRelationshipResponseDetailed - fuzzes detailed relationship parsing
// ============================================================================

func FuzzParseRelationshipResponseDetailed(f *testing.F) {
	// Seed corpus with interesting inputs
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}]}`)
	f.Add(``)
	f.Add(`{"relationships": []}`)
	f.Add(`not json at all`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "invalid_type", "confidence": 0.9}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 0.9}, {"from": "c", "to": "d", "type": "unknown", "confidence": 0.8}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": 1.5}]}`)
	f.Add(`{"relationships": [{"from": "a", "to": "b", "type": "employed_by", "confidence": -0.1}]}`)
	f.Add(`{{{`)
	f.Add("```json\n{\"relationships\": [{\"from\": \"a\", \"to\": \"b\", \"type\": \"employed_by\", \"confidence\": 0.9}]}\n```")

	f.Fuzz(func(t *testing.T, input string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseRelationshipResponseDetailed panicked on input %q: %v", input, r)
			}
		}()
		_, _, _ = ParseRelationshipResponseDetailed(input)
	})
}
