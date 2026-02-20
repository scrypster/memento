package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MultiCallTest represents a test of multiple LLM calls on the same payload
type MultiCallTest struct {
	Name        string
	Description string
	Calls       []PromptCall
	TestContent string
}

// PromptCall represents a single LLM call with its prompt variation
type PromptCall struct {
	Name       string
	Purpose    string // What we're extracting (entities, relationships, keywords, etc.)
	PromptFunc func(content string) string
}

// CallResult represents the result of a single LLM call
type CallResult struct {
	CallName  string
	Purpose   string
	Output    string
	ParsedOK  bool
	Duration  time.Duration
	ErrorMsg  string
}

// TestResult represents results from a complete multi-call test
type TestResult struct {
	TestName  string
	TotalTime time.Duration
	Results   []CallResult
	Success   bool
}

// MultiCallTestFunc is a function type for running multi-call tests
type MultiCallTestFunc func(test MultiCallTest, client *OllamaClient) *TestResult

// Entity is a simple entity type for testing
type Entity struct {
	Name string
	Type string
}

// Define test payloads for comprehensive testing
var TestPayloads = map[string]string{
	"simple": `Alice works at Google using Python. She manages the DataPipeline project.`,

	"medium": `MJ Bonanno is the founder of Scrypster and is married to Norma Bonanno.
Norma works at Google as a senior engineer. Their son Rosario is a software engineer.
The Memento project uses Python and PostgreSQL for its backend. MJ specializes in Go.`,

	"complex": `Alice and Bob are colleagues at TechCorp working on the Memento project.
Alice uses Python and manages technical infrastructure. Bob specializes in Go and backend development.
Both use Docker for deployment. The Memento project depends on PostgreSQL for data storage.
They also collaborate with Charlie from the DevOps team. The entire stack includes React for frontend,
Node.js for services, Kubernetes for orchestration, and Redis for caching.
Alice is married to Diana, who works at Microsoft. Bob's brother Charlie also codes in Rust.
The project was created in 2022 and has 50+ contributors from various organizations.`,
}

// Multi-call test configurations
var MultiCallTests = []MultiCallTest{
	{
		Name:        "Entity Only",
		Description: "Single call extracting only entities",
		Calls: []PromptCall{
			{
				Name:    "V1_Entities",
				Purpose: "entity_extraction",
				PromptFunc: func(content string) string {
					return EntityExtractionPrompt(content)
				},
			},
		},
		TestContent: TestPayloads["medium"],
	},
	{
		Name:        "Two-Call Split (Entities + Relationships)",
		Description: "Call 1: Extract entities, Call 2: Extract relationships",
		Calls: []PromptCall{
			{
				Name:    "V1_Entities",
				Purpose: "entity_extraction",
				PromptFunc: func(content string) string {
					return EntityExtractionPrompt(content)
				},
			},
			{
				Name:    "V1_Relationships",
				Purpose: "relationship_extraction",
				PromptFunc: func(content string) string {
					// Simplified relationship prompt for testing
					return fmt.Sprintf(`Extract relationships between entities in this text.
Types: works_for, created, uses, owns, manages, sibling_of, parent_of, child_of, married_to, colleague_of, works_on, depends_on

Text: %s

Return JSON only: {"relationships":[{"from":"X","to":"Y","type":"works_for","confidence":0.9}]}`, content)
				},
			},
		},
		TestContent: TestPayloads["medium"],
	},
	{
		Name:        "Three-Call Pipeline (Entities + Relationships + Keywords)",
		Description: "Call 1: Entities, Call 2: Relationships, Call 3: Keywords/Summary",
		Calls: []PromptCall{
			{
				Name:    "V1_Entities",
				Purpose: "entity_extraction",
				PromptFunc: func(content string) string {
					return EntityExtractionPrompt(content)
				},
			},
			{
				Name:    "V1_Relationships",
				Purpose: "relationship_extraction",
				PromptFunc: func(content string) string {
					return fmt.Sprintf(`Extract relationships between entities.
Types: works_for, created, uses, owns, manages, sibling_of, parent_of, child_of, married_to, colleague_of, works_on, depends_on

Text: %s

JSON: {"relationships":[{"from":"X","to":"Y","type":"works_for","confidence":0.9}]}`, content)
				},
			},
			{
				Name:    "Keywords",
				Purpose: "keyword_extraction",
				PromptFunc: func(content string) string {
					return fmt.Sprintf(`Extract 5-10 important keywords from this text.
Return JSON only: {"keywords":["word1","word2"]}

Text: %s

JSON:`, content)
				},
			},
		},
		TestContent: TestPayloads["complex"],
	},
	{
		Name:        "Minimal Entity Extraction",
		Description: "Ultra-concise entity extraction prompt",
		Calls: []PromptCall{
			{
				Name:    "Minimal_Entities",
				Purpose: "entity_extraction",
				PromptFunc: func(content string) string {
					return fmt.Sprintf(`Extract: person|organization|tool|project
JSON: {"entities":[{"name":"X","type":"person","description":"...","confidence":0.9}]}

Text: %s

JSON:`, content)
				},
			},
		},
		TestContent: TestPayloads["medium"],
	},
	{
		Name:        "Verbose Entity Extraction",
		Description: "Detailed entity extraction with many examples",
		Calls: []PromptCall{
			{
				Name:    "Verbose_Entities",
				Purpose: "entity_extraction",
				PromptFunc: func(content string) string {
					return fmt.Sprintf(`Extract all named entities from the text.

DETAILED ENTITY TYPES:

person: Named individual human
  Examples: Alice, Bob, "Dr. Jane Smith", CEO John Davis, MJ Bonanno, Norma
  NOT person: "team", "group", "people", titles without names

organization: Company, institution, or group entity
  Examples: Google, MIT, Red Cross, "Acme Inc", NASA, Microsoft, TechCorp
  NOT organization: individual people, software tools, project names, teams

tool: Software, library, framework, technology, programming language
  Examples: Python, Kubernetes, React, Docker, PostgreSQL, Go, Java, npm, Node.js, Redis
  NOT tool: project names, people, companies, generic "software"

project: Specific initiative, product, or named work
  Examples: ProjectX, Apollo, DataPipeline, Ubuntu, Memento, Kubernetes
  NOT project: tools, frameworks, organizations, generic words

Confidence must be 0.6-0.99.
Return ONLY valid JSON:
{"entities":[{"name":"X","type":"person","description":"...","confidence":0.9}]}

Text: %s

JSON:`, content)
				},
			},
		},
		TestContent: TestPayloads["medium"],
	},
}

// RunMultiCallTest executes a multi-call test and returns results
func RunMultiCallTest(test MultiCallTest, client *OllamaClient) *TestResult {
	startTime := time.Now()
	results := make([]CallResult, 0)
	success := true

	for _, call := range test.Calls {
		callStart := time.Now()
		prompt := call.PromptFunc(test.TestContent)

		// Execute LLM call
		response, _ := client.Complete(context.Background(), prompt)
		callDuration := time.Since(callStart)

		// Parse based on call type
		var parseErr error
		var parsedOK bool

		switch call.Purpose {
		case "entity_extraction":
			_, parseErr = ParseEntityResponse(response)
			parsedOK = parseErr == nil
		case "relationship_extraction":
			_, parseErr = ParseRelationshipResponse(response)
			parsedOK = parseErr == nil
		case "keyword_extraction":
			_, parseErr = ParseKeywordResponse(response)
			parsedOK = parseErr == nil
		default:
			parsedOK = len(response) > 0
		}

		if parseErr != nil {
			success = false
		}

		result := CallResult{
			CallName: call.Name,
			Purpose:  call.Purpose,
			Output:   truncateOutput(response, 200),
			ParsedOK: parsedOK,
			Duration: callDuration,
		}

		if parseErr != nil {
			result.ErrorMsg = parseErr.Error()
		}

		results = append(results, result)
	}

	totalTime := time.Since(startTime)

	return &TestResult{
		TestName:  test.Name,
		TotalTime: totalTime,
		Results:   results,
		Success:   success,
	}
}

// truncateOutput returns first N characters of output
func truncateOutput(output string, maxLen int) string {
	if len(output) > maxLen {
		return output[:maxLen] + "..."
	}
	return output
}

// FormatTestResults returns a formatted string of test results
func FormatTestResults(result *TestResult) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "\n=== Test: %s ===\n", result.TestName)
	fmt.Fprintf(&sb, "Total Time: %.2fs\n", result.TotalTime.Seconds())
	overallStatus := "❌ FAIL"
	if result.Success {
		overallStatus = "✅ PASS"
	}
	fmt.Fprintf(&sb, "Overall Status: %s\n", overallStatus)

	sb.WriteString("\nCalls:\n")
	for i, cr := range result.Results {
		fmt.Fprintf(&sb, "\n%d. %s (%s)\n", i+1, cr.CallName, cr.Purpose)
		callStatus := "❌ Parse Error"
		if cr.ParsedOK {
			callStatus = "✅ Parsed OK"
		}
		fmt.Fprintf(&sb, "   Status: %s\n", callStatus)
		fmt.Fprintf(&sb, "   Duration: %.2fs\n", cr.Duration.Seconds())
		if cr.ErrorMsg != "" {
			fmt.Fprintf(&sb, "   Error: %s\n", cr.ErrorMsg)
		}
		fmt.Fprintf(&sb, "   Output: %s\n", cr.Output)
	}

	return sb.String()
}
