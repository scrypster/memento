package importer

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// ParsedFile represents a single Markdown file that has been parsed.
type ParsedFile struct {
	// Path is the absolute filesystem path to the file.
	Path string

	// RelativePath is the path relative to the import root directory.
	RelativePath string

	// Title is derived from the filename (without extension) or the first H1 heading.
	Title string

	// Content is the raw Markdown body (frontmatter stripped).
	Content string

	// Frontmatter holds the parsed YAML frontmatter key/value pairs.
	Frontmatter map[string]interface{}

	// Tags is the merged set of tags from frontmatter and inline #tags.
	Tags []string

	// Category is the primary category extracted from frontmatter or directory structure.
	Category string

	// Domain is derived from the top-level directory within the import root.
	Domain string

	// WikiLinks are all [[link]] targets referenced by this file.
	WikiLinks []WikiLink

	// Timestamp is from the frontmatter "date" field, or zero if absent.
	Timestamp time.Time
}

// ParseMarkdownFile parses a single Markdown file's content.
// relativePath is used to derive domain/category from the directory structure.
func ParseMarkdownFile(content []byte, absolutePath, relativePath string) (*ParsedFile, error) {
	text := string(content)

	// Derive domain from first directory component of relativePath.
	domain := domainFromPath(relativePath)
	category := categoryFromPath(relativePath)
	title := titleFromPath(relativePath)

	// Split frontmatter from body.
	fm, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, fmt.Errorf("frontmatter parse error in %s: %w", relativePath, err)
	}

	// Extract structured fields from frontmatter.
	tags := extractTags(fm)
	ts := extractTimestamp(fm)
	fmCategory := extractString(fm, "category", "")
	fmDomain := extractString(fm, "domain", "")
	fmTitle := extractString(fm, "title", "")

	if fmCategory != "" {
		category = fmCategory
	}
	if fmDomain != "" {
		domain = fmDomain
	}
	if fmTitle != "" {
		title = fmTitle
	}

	// Also scan for H1 heading as title if not in frontmatter.
	if title == "" || title == titleFromPath(relativePath) {
		if h1 := extractH1(body); h1 != "" {
			title = h1
		}
	}

	// Extract inline #hashtags from body and merge with frontmatter tags.
	inlineTags := extractInlineTags(body)
	tags = mergeTags(tags, inlineTags)

	// Extract wiki links.
	wikiLinks := ExtractWikiLinks(body)

	// Build the readable content block: title + body (with wiki links converted to plain text).
	readableBody := StripWikiLinks(body)
	finalContent := buildContent(title, readableBody, domain, category)

	return &ParsedFile{
		Path:         absolutePath,
		RelativePath: relativePath,
		Title:        title,
		Content:      finalContent,
		Frontmatter:  fm,
		Tags:         tags,
		Category:     category,
		Domain:       domain,
		WikiLinks:    wikiLinks,
		Timestamp:    ts,
	}, nil
}

// splitFrontmatter separates YAML frontmatter (between --- delimiters) from
// the Markdown body. Returns empty map and full text when no frontmatter found.
func splitFrontmatter(text string) (map[string]interface{}, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(text))

	// Collect lines.
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) == 0 {
		return map[string]interface{}{}, text, nil
	}

	// Frontmatter must start with "---" on the first line.
	if strings.TrimSpace(lines[0]) != "---" {
		return map[string]interface{}{}, text, nil
	}

	// Find closing "---".
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}

	if closeIdx == -1 {
		// No closing delimiter - treat entire file as body.
		return map[string]interface{}{}, text, nil
	}

	// Parse frontmatter YAML.
	fmText := strings.Join(lines[1:closeIdx], "\n")
	fm := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return map[string]interface{}{}, text, fmt.Errorf("invalid YAML: %w", err)
	}

	body := strings.Join(lines[closeIdx+1:], "\n")
	return fm, body, nil
}

// domainFromPath returns the top-level directory as the domain name,
// falling back to "import" when the file is at root level.
func domainFromPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 1 {
		return sanitizeSegment(parts[0])
	}
	return "import"
}

// categoryFromPath returns the second directory segment as a category,
// or "" when the file is at most one level deep.
func categoryFromPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 2 {
		return sanitizeSegment(parts[1])
	}
	return ""
}

// titleFromPath derives a human-readable title from the file name (no extension).
func titleFromPath(rel string) string {
	base := filepath.Base(rel)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	// Replace underscores/dashes with spaces.
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.TrimSpace(name)
}

// extractH1 returns the text of the first ATX heading (# ...) found in the body.
func extractH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

// extractTags reads tags from frontmatter. Handles both list and string forms.
func extractTags(fm map[string]interface{}) []string {
	raw, ok := fm["tags"]
	if !ok {
		return nil
	}

	switch v := raw.(type) {
	case []interface{}:
		var tags []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				tags = append(tags, s)
			}
		}
		return tags
	case string:
		if v == "" {
			return nil
		}
		// Comma-separated tags in a single string.
		var tags []string
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}
	return nil
}

// extractTimestamp reads a date field from frontmatter and attempts several
// common layouts.
func extractTimestamp(fm map[string]interface{}) time.Time {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"January 2, 2006",
		"Jan 2, 2006",
	}

	for _, key := range []string{"date", "created", "created_at", "updated_at"} {
		raw, ok := fm[key]
		if !ok {
			continue
		}
		var s string
		switch v := raw.(type) {
		case string:
			s = v
		case time.Time:
			return v
		default:
			s = fmt.Sprintf("%v", v)
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// extractString pulls a string value from frontmatter by key with a default.
func extractString(fm map[string]interface{}, key, defaultVal string) string {
	v, ok := fm[key]
	if !ok {
		return defaultVal
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return defaultVal
}

// inlineTagRe finds #hashtag patterns in body text.
var inlineTagRe = regexp.MustCompile(`(?:^|\s)#([A-Za-z][A-Za-z0-9_/-]*)`)

// extractInlineTags finds #hashtag patterns in body text.
func extractInlineTags(body string) []string {
	matches := inlineTagRe.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)
	var tags []string
	for _, m := range matches {
		tag := strings.TrimSpace(m[1])
		lower := strings.ToLower(tag)
		if !seen[lower] {
			seen[lower] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// mergeTags combines two tag slices deduplicating by lowercase value.
func mergeTags(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, tag := range append(a, b...) {
		lower := strings.ToLower(tag)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, tag)
		}
	}
	return result
}

// sanitizeSegment makes a path segment safe to use as a domain/category.
func sanitizeSegment(s string) string {
	s = strings.TrimSpace(s)
	// Replace spaces and special chars with hyphens.
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// buildContent assembles the final memory content string with context.
// It avoids prepending a duplicate title heading when the body already starts
// with an H1 that matches the title.
func buildContent(title, body, domain, category string) string {
	body = strings.TrimSpace(body)

	var parts []string

	// Only add a title heading when the body does not already open with one.
	bodyStartsWithH1 := strings.HasPrefix(body, "# ")
	if title != "" && !bodyStartsWithH1 {
		parts = append(parts, fmt.Sprintf("# %s", title))
	}

	if domain != "" && domain != "import" {
		meta := fmt.Sprintf("Domain: %s", domain)
		if category != "" {
			meta += fmt.Sprintf(" / Category: %s", category)
		}
		parts = append(parts, meta)
	}

	if body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}
