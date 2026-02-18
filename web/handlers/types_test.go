package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskAPIKey_HandlesEmptyString(t *testing.T) {
	masked := MaskAPIKey("")
	if masked != "" {
		t.Errorf("Expected empty string, got %q", masked)
	}
}

func TestMaskAPIKey_HandlesShortKey(t *testing.T) {
	masked := MaskAPIKey("short")
	if masked != "***" {
		t.Errorf("Expected '***', got %q", masked)
	}
}

func TestMaskAPIKey_MasksLongKey(t *testing.T) {
	masked := MaskAPIKey("sk-proj-abcdefghijklmnopqrstuvwxyz1234567890")
	expected := "sk-proj...7890"
	if masked != expected {
		t.Errorf("Expected %q, got %q", expected, masked)
	}
	if strings.Contains(masked, "abcdefgh") {
		t.Errorf("Masked key should not contain middle portion 'abcdefgh', got %q", masked)
	}
}

func TestErrorResponse_MarshalJSON(t *testing.T) {
	err := &ErrorResponse{
		Error:   "Memory not found",
		Code:    "NOT_FOUND",
		Details: map[string]interface{}{"id": "mem:123"},
	}

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("Failed to marshal ErrorResponse: %v", jsonErr)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, "Memory not found") {
		t.Errorf("Expected JSON to contain 'Memory not found', got %q", jsonStr)
	}
	if !strings.Contains(jsonStr, "NOT_FOUND") {
		t.Errorf("Expected JSON to contain 'NOT_FOUND', got %q", jsonStr)
	}
	if !strings.Contains(jsonStr, "mem:123") {
		t.Errorf("Expected JSON to contain 'mem:123', got %q", jsonStr)
	}
}
