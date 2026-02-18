// Package importer provides parsers for importing knowledge bases from Obsidian,
// Notion exports, and generic Markdown folders into the Memento memory system.
package importer

import (
	"regexp"
	"strings"
)

// wikilinkRe matches [[link]] and [[link|alias]] patterns.
var wikilinkRe = regexp.MustCompile(`\[\[([^\[\]|]+?)(?:\|([^\[\]]+?))?\]\]`)

// WikiLink represents a parsed [[wiki-link]] from Markdown content.
type WikiLink struct {
	// Target is the note/page name being linked to.
	Target string

	// Alias is the display text (if [[target|alias]] syntax is used).
	// Empty when no alias is specified.
	Alias string

	// Raw is the full original [[...]] text.
	Raw string
}

// ExtractWikiLinks finds all [[wiki-link]] patterns in the given content and
// returns them as a deduplicated slice ordered by first appearance.
func ExtractWikiLinks(content string) []WikiLink {
	matches := wikilinkRe.FindAllStringSubmatch(content, -1)

	seen := make(map[string]bool)
	var links []WikiLink

	for _, m := range matches {
		raw := m[0]
		target := strings.TrimSpace(m[1])
		alias := strings.TrimSpace(m[2])

		// Deduplicate by target name (case-insensitive).
		key := strings.ToLower(target)
		if seen[key] {
			continue
		}
		seen[key] = true

		links = append(links, WikiLink{
			Target: target,
			Alias:  alias,
			Raw:    raw,
		})
	}

	return links
}

// StripWikiLinks replaces [[wiki-links]] in content with plain text.
// If the link has an alias, the alias is used; otherwise the target name is used.
func StripWikiLinks(content string) string {
	return wikilinkRe.ReplaceAllStringFunc(content, func(match string) string {
		parts := wikilinkRe.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			return strings.TrimSpace(parts[2])
		}
		return strings.TrimSpace(parts[1])
	})
}
