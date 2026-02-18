package types

// ClassificationSchema represents a category with its available classifications.
// This enables each connection to have custom classification options while
// providing sensible system defaults.
type ClassificationSchema struct {
	// Category name (e.g., "Software Development", "Personal Assistant")
	Category string `json:"category"`

	// Description of the category's purpose
	Description string `json:"description,omitempty"`

	// Classifications available within this category (editable per connection)
	Classifications []Classification `json:"classifications"`

	// IsSystem indicates this is a system-provided default (can be overridden)
	IsSystem bool `json:"is_system"`
}

// Classification represents a single classification option within a category
type Classification struct {
	// Unique identifier (e.g., "architecture-design", "bug-issue")
	ID string `json:"id"`

	// Display name (e.g., "Architecture & Design", "Bug & Issue")
	Name string `json:"name"`

	// Description of what this classification covers
	Description string `json:"description,omitempty"`

	// Keywords that help the LLM identify this classification
	Keywords []string `json:"keywords,omitempty"`

	// Color/icon hint for UI display (optional)
	Icon string `json:"icon,omitempty"`
}

// DefaultClassificationSchemas provides system default categories with classifications
// organized by domain. Each connection can customize these or add new ones.
func DefaultClassificationSchemas() []ClassificationSchema {
	return []ClassificationSchema{
		{
			Category:    "Software Development",
			Description: "Software engineering, coding, and technical implementation",
			IsSystem:    true,
			Classifications: []Classification{
				{
					ID:          "architecture-design",
					Name:        "Architecture & Design",
					Description: "System design, architecture decisions, design patterns",
					Keywords:    []string{"architecture", "design", "pattern", "system", "structure"},
					Icon:        "🏗️",
				},
				{
					ID:          "bug-issue",
					Name:        "Bug & Issue",
					Description: "Bug reports, issues, and problems to fix",
					Keywords:    []string{"bug", "issue", "error", "problem", "fix"},
					Icon:        "🐛",
				},
				{
					ID:          "feature-request",
					Name:        "Feature Request",
					Description: "New features, enhancements, and improvements",
					Keywords:    []string{"feature", "enhancement", "improvement", "new", "request"},
					Icon:        "✨",
				},
				{
					ID:          "code-review",
					Name:        "Code Review",
					Description: "Code review comments, feedback, and suggestions",
					Keywords:    []string{"review", "code", "feedback", "suggestion", "improvement"},
					Icon:        "👀",
				},
				{
					ID:          "testing-qa",
					Name:        "Testing & QA",
					Description: "Testing strategies, test cases, quality assurance",
					Keywords:    []string{"test", "qa", "quality", "testing", "coverage"},
					Icon:        "✅",
				},
				{
					ID:          "performance",
					Name:        "Performance & Optimization",
					Description: "Performance improvements, optimization, benchmarking",
					Keywords:    []string{"performance", "optimization", "speed", "efficiency", "benchmark"},
					Icon:        "⚡",
				},
				{
					ID:          "security",
					Name:        "Security & Vulnerability",
					Description: "Security issues, vulnerabilities, hardening",
					Keywords:    []string{"security", "vulnerability", "threat", "attack", "safe"},
					Icon:        "🔒",
				},
				{
					ID:          "refactoring",
					Name:        "Refactoring",
					Description: "Code refactoring, cleanup, technical debt",
					Keywords:    []string{"refactor", "cleanup", "debt", "technical", "improve"},
					Icon:        "♻️",
				},
				{
					ID:          "documentation",
					Name:        "Documentation",
					Description: "Code documentation, guides, comments",
					Keywords:    []string{"documentation", "doc", "guide", "readme", "comment"},
					Icon:        "📚",
				},
			},
		},
		{
			Category:    "Project Management",
			Description: "Project planning, tracking, and execution",
			IsSystem:    true,
			Classifications: []Classification{
				{
					ID:          "planning",
					Name:        "Planning & Scope",
					Description: "Project planning, scope definition, requirements",
					Keywords:    []string{"plan", "scope", "requirement", "spec", "definition"},
					Icon:        "📋",
				},
				{
					ID:          "task-assignment",
					Name:        "Task & Assignment",
					Description: "Task creation, assignment, and distribution",
					Keywords:    []string{"task", "assign", "work", "todo", "assignment"},
					Icon:        "📝",
				},
				{
					ID:          "timeline-deadline",
					Name:        "Timeline & Deadline",
					Description: "Dates, milestones, deadlines, schedules",
					Keywords:    []string{"deadline", "timeline", "date", "schedule", "milestone"},
					Icon:        "📅",
				},
				{
					ID:          "resource-allocation",
					Name:        "Resource Allocation",
					Description: "Resource planning, budgets, capacity",
					Keywords:    []string{"resource", "budget", "capacity", "allocation", "availability"},
					Icon:        "💰",
				},
				{
					ID:          "risk-issue",
					Name:        "Risk & Issue",
					Description: "Risks, issues, blockers, dependencies",
					Keywords:    []string{"risk", "issue", "blocker", "depend", "problem"},
					Icon:        "⚠️",
				},
				{
					ID:          "progress-status",
					Name:        "Progress & Status",
					Description: "Status updates, progress reports, metrics",
					Keywords:    []string{"progress", "status", "update", "metric", "report"},
					Icon:        "📊",
				},
				{
					ID:          "retrospective",
					Name:        "Retrospective & Learning",
					Description: "Lessons learned, retrospectives, improvements",
					Keywords:    []string{"retrospective", "learn", "improve", "lesson", "feedback"},
					Icon:        "🔍",
				},
			},
		},
		{
			Category:    "Business & Operations",
			Description: "Business strategy, operations, and management",
			IsSystem:    true,
			Classifications: []Classification{
				{
					ID:          "strategy-vision",
					Name:        "Strategy & Vision",
					Description: "Business strategy, vision, goals, roadmap",
					Keywords:    []string{"strategy", "vision", "goal", "roadmap", "objective"},
					Icon:        "🎯",
				},
				{
					ID:          "process-workflow",
					Name:        "Process & Workflow",
					Description: "Business processes, workflows, procedures",
					Keywords:    []string{"process", "workflow", "procedure", "step", "flow"},
					Icon:        "🔄",
				},
				{
					ID:          "team-people",
					Name:        "Team & People",
					Description: "Team information, personnel, organization",
					Keywords:    []string{"team", "people", "person", "org", "staff"},
					Icon:        "👥",
				},
				{
					ID:          "meeting-discussion",
					Name:        "Meeting & Discussion",
					Description: "Meeting notes, discussions, conversations",
					Keywords:    []string{"meeting", "discussion", "conversation", "talk", "sync"},
					Icon:        "💬",
				},
				{
					ID:          "decision-policy",
					Name:        "Decision & Policy",
					Description: "Business decisions, policies, guidelines",
					Keywords:    []string{"decision", "policy", "guideline", "rule", "standard"},
					Icon:        "📋",
				},
				{
					ID:          "vendor-partnership",
					Name:        "Vendor & Partnership",
					Description: "Vendor information, partnerships, contracts",
					Keywords:    []string{"vendor", "partner", "contract", "deal", "relationship"},
					Icon:        "🤝",
				},
				{
					ID:          "legal-compliance",
					Name:        "Legal & Compliance",
					Description: "Legal matters, compliance, regulatory requirements",
					Keywords:    []string{"legal", "compliance", "regulation", "law", "requirement"},
					Icon:        "⚖️",
				},
			},
		},
		{
			Category:    "Research & Learning",
			Description: "Education, research, knowledge building",
			IsSystem:    true,
			Classifications: []Classification{
				{
					ID:          "article-paper",
					Name:        "Article & Paper",
					Description: "Articles, research papers, publications",
					Keywords:    []string{"article", "paper", "research", "publication", "study"},
					Icon:        "📰",
				},
				{
					ID:          "tutorial-guide",
					Name:        "Tutorial & Guide",
					Description: "Tutorials, how-to guides, documentation",
					Keywords:    []string{"tutorial", "guide", "how-to", "learn", "instruction"},
					Icon:        "🎓",
				},
				{
					ID:          "experiment-test",
					Name:        "Experiment & Test",
					Description: "Experiments, testing, POCs, investigations",
					Keywords:    []string{"experiment", "test", "poc", "investigate", "trial"},
					Icon:        "🧪",
				},
				{
					ID:          "tool-technology",
					Name:        "Tool & Technology",
					Description: "Tools, technologies, frameworks, libraries",
					Keywords:    []string{"tool", "technology", "framework", "library", "platform"},
					Icon:        "🔧",
				},
				{
					ID:          "best-practice",
					Name:        "Best Practice",
					Description: "Best practices, patterns, recommendations",
					Keywords:    []string{"best", "practice", "pattern", "recommend", "standard"},
					Icon:        "⭐",
				},
				{
					ID:          "concept-theory",
					Name:        "Concept & Theory",
					Description: "Concepts, theories, principles, ideas",
					Keywords:    []string{"concept", "theory", "principle", "idea", "model"},
					Icon:        "💡",
				},
			},
		},
		{
			Category:    "Personal Assistant",
			Description: "Personal productivity, life management, goals",
			IsSystem:    true,
			Classifications: []Classification{
				{
					ID:          "health-wellness",
					Name:        "Health & Wellness",
					Description: "Health, fitness, wellness, medical",
					Keywords:    []string{"health", "wellness", "fitness", "medical", "exercise"},
					Icon:        "💪",
				},
				{
					ID:          "finances-budget",
					Name:        "Finances & Budget",
					Description: "Financial planning, budgets, investments",
					Keywords:    []string{"finance", "budget", "money", "invest", "expense"},
					Icon:        "💵",
				},
				{
					ID:          "personal-goals",
					Name:        "Personal Goals",
					Description: "Personal goals, aspirations, development",
					Keywords:    []string{"goal", "aspiration", "develop", "improve", "growth"},
					Icon:        "🎯",
				},
				{
					ID:          "learning-development",
					Name:        "Learning & Development",
					Description: "Personal learning, skill development, education",
					Keywords:    []string{"learn", "develop", "skill", "education", "course"},
					Icon:        "📚",
				},
				{
					ID:          "travel-plans",
					Name:        "Travel & Plans",
					Description: "Travel planning, trips, adventures",
					Keywords:    []string{"travel", "trip", "plan", "vacation", "adventure"},
					Icon:        "✈️",
				},
				{
					ID:          "family-relationships",
					Name:        "Family & Relationships",
					Description: "Family information, relationships, social",
					Keywords:    []string{"family", "relationship", "friend", "social", "contact"},
					Icon:        "👨‍👩‍👧",
				},
				{
					ID:          "entertainment-hobbies",
					Name:        "Entertainment & Hobbies",
					Description: "Hobbies, entertainment, recreation",
					Keywords:    []string{"hobby", "entertainment", "recreation", "interest", "fun"},
					Icon:        "🎮",
				},
			},
		},
		{
			Category:    "Communication & Collaboration",
			Description: "Messages, collaboration, feedback",
			IsSystem:    true,
			Classifications: []Classification{
				{
					ID:          "message-notification",
					Name:        "Message & Notification",
					Description: "Messages, notifications, alerts",
					Keywords:    []string{"message", "notification", "alert", "notification", "ping"},
					Icon:        "📬",
				},
				{
					ID:          "email-correspondence",
					Name:        "Email & Correspondence",
					Description: "Email threads, correspondence, formal communication",
					Keywords:    []string{"email", "correspondence", "mail", "formal", "communication"},
					Icon:        "📧",
				},
				{
					ID:          "meeting-notes",
					Name:        "Meeting Notes",
					Description: "Meeting notes, action items, decisions",
					Keywords:    []string{"meeting", "notes", "action", "decision", "attendee"},
					Icon:        "📝",
				},
				{
					ID:          "feedback-review",
					Name:        "Feedback & Review",
					Description: "Feedback, reviews, constructive criticism",
					Keywords:    []string{"feedback", "review", "critique", "comment", "suggestion"},
					Icon:        "💭",
				},
				{
					ID:          "announcement-update",
					Name:        "Announcement & Update",
					Description: "Announcements, updates, news",
					Keywords:    []string{"announce", "update", "news", "information", "alert"},
					Icon:        "📢",
				},
			},
		},
	}
}

// DefaultClassificationMap creates a quick lookup map of all default classifications
// organized by category for easy access during enrichment
func DefaultClassificationMap() map[string][]Classification {
	schemas := DefaultClassificationSchemas()
	result := make(map[string][]Classification)
	for _, schema := range schemas {
		result[schema.Category] = schema.Classifications
	}
	return result
}
