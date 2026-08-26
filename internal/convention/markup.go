package convention

import (
	"strings"

	"golang.org/x/net/html"
)

// tearoutNesting reports a tag that never closes, closes nothing, or closes the
// wrong element.
//
// A browser repairs all three and renders something plausible, so the fault
// stays invisible until a later edit lands inside the wrong element.
func tearoutNesting(t Tearout) []Finding {
	var out []Finding
	for _, page := range t.Pages {
		var open []markup
		for _, m := range tokens(page.Body) {
			if m.isVoid() {
				continue
			}
			switch m.Kind {
			case html.StartTagToken:
				open = append(open, m)
			case html.EndTagToken:
				if len(open) == 0 {
					out = append(out, Finding{
						At:    tearoutAt(page, m.Line),
						Check: "tearout-nesting",
						What:  "</" + m.Tag + "> closes nothing",
					})
					continue
				}
				last := open[len(open)-1]
				if last.Tag != m.Tag {
					out = append(out, Finding{
						At:    tearoutAt(page, m.Line),
						Check: "tearout-nesting",
						What:  "</" + m.Tag + "> closes <" + last.Tag + ">",
					})
				}
				open = open[:len(open)-1]
			}
		}
		for _, m := range open {
			out = append(out, Finding{
				At:    tearoutAt(page, m.Line),
				Check: "tearout-nesting",
				What:  "<" + m.Tag + "> is never closed",
			})
		}
	}
	return out
}

// tearoutClasses reports a class used in markup that no stylesheet defines.
//
// The element still renders, unstyled, so the page looks close enough to right
// that nobody goes looking for the missing rule.
func tearoutClasses(t Tearout) []Finding {
	defined := map[string]struct{}{}
	for _, m := range cssClass.FindAllStringSubmatch(t.CSS, -1) {
		defined[m[1]] = struct{}{}
	}
	var out []Finding
	for _, page := range t.Pages {
		seen := map[string]struct{}{}
		for _, m := range tokens(page.Body) {
			for _, name := range m.classes() {
				if _, ok := defined[name]; ok {
					continue
				}
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				out = append(out, Finding{
					At:    tearoutAt(page, m.Line),
					Check: "tearout-classes",
					What:  "class ." + name + " is used but no stylesheet defines it",
				})
			}
		}
	}
	return out
}

// tearoutFragments reports a fragment link pointing at an id the page lacks.
//
// The tearouts expand with :target, so a link to a missing id is a control that
// does nothing at all when it is tapped.
func tearoutFragments(t Tearout) []Finding {
	var out []Finding
	for _, page := range t.Pages {
		ids := map[string]struct{}{}
		var frags []markup
		for _, m := range tokens(page.Body) {
			if id := m.Attr["id"]; id != "" {
				ids[id] = struct{}{}
			}
			if href := m.Attr["href"]; strings.HasPrefix(href, "#") && len(href) > 1 {
				frags = append(frags, m)
			}
		}
		seen := map[string]struct{}{}
		for _, m := range frags {
			frag := m.Attr["href"][1:]
			if _, ok := ids[frag]; ok {
				continue
			}
			if _, ok := seen[frag]; ok {
				continue
			}
			seen[frag] = struct{}{}
			out = append(out, Finding{
				At:    tearoutAt(page, m.Line),
				Check: "tearout-fragments",
				What:  "href #" + frag + " points at no id on the page",
			})
		}
	}
	return out
}

// tearoutIcons reports an icon span that draws nothing, or one that draws text.
//
// A glyph comes from an i-<name> class whose mask lives in app.css. A span
// carrying i with no i-<name> draws nothing at all, and text inside one renders
// beside the glyph rather than instead of it. Both look plausible until the page
// is opened in an engine that renders them.
func tearoutIcons(t Tearout) []Finding {
	var out []Finding
	for _, page := range t.Pages {
		spans(page.Body, "i", func(m markup, inside bool) {
			if m.Kind == html.TextToken {
				text := strings.TrimSpace(m.Text)
				if !inside || text == "" {
					return
				}
				out = append(out, Finding{
					At:    tearoutAt(page, m.Line),
					Check: "tearout-icons",
					What:  "text inside an icon span: " + text,
				})
				return
			}
			if m.Kind != html.StartTagToken || !m.hasClass("i") || hasGlyph(m) {
				return
			}
			out = append(out, Finding{
				At:    tearoutAt(page, m.Line),
				Check: "tearout-icons",
				What:  "class i with no i-<name> beside it draws nothing: " + m.Attr["class"],
			})
		})
	}
	return out
}

// hasGlyph reports whether a tag names which glyph to draw.
func hasGlyph(m markup) bool {
	for _, name := range m.classes() {
		if strings.HasPrefix(name, "i-") {
			return true
		}
	}
	return false
}

// tearoutAgents reports an agent name written into prose on a page that also
// displays it.
//
// The displayed copy comes from an .agent span. A second copy in a sentence is
// not kept in step with it, so renaming the agent leaves the sentence behind.
func tearoutAgents(t Tearout) []Finding {
	shown := map[string]struct{}{}
	for _, page := range t.Pages {
		for _, name := range agentNames(page.Body) {
			shown[name] = struct{}{}
		}
	}
	var out []Finding
	for _, page := range t.Pages {
		prose := proseOf(page.Body)
		for _, name := range sortedSet(shown) {
			if !strings.Contains(prose, name) {
				continue
			}
			out = append(out, Finding{
				At:    page.Name,
				Check: "tearout-agents",
				What:  "prose names the agent " + name + ", which the page also displays",
			})
		}
	}
	return out
}

// agentNames lists the agents a page displays in an .agent span.
//
// A name beginning with ~ is a placeholder standing in for one, not an agent.
func agentNames(body string) []string {
	var out []string
	spans(body, "agent", func(m markup, inside bool) {
		if m.Kind != html.TextToken || !inside {
			return
		}
		name := strings.TrimSpace(m.Text)
		if name == "" || strings.HasPrefix(name, "~") {
			return
		}
		out = append(out, name)
	})
	return out
}

// proseOf is a page's character data with the .agent spans left out, which is
// every place a name would be a second copy rather than the displayed one.
func proseOf(body string) string {
	var sb strings.Builder
	spans(body, "agent", func(m markup, inside bool) {
		if m.Kind != html.TextToken || inside {
			return
		}
		sb.WriteString(m.Text)
		sb.WriteString(" ")
	})
	return sb.String()
}
