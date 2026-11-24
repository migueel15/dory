package app

import (
	"html"
	"sort"
	"strings"
)

// Messages Dory writes for the user (to define or implement a task, review a
// pull request…) wrap their instructions in a <dory-context> block. The agent
// reads the whole text; the thread view shows a card with the kind, a link to
// what it is about and only the user's own words, with the context folded.
// The web reads the same format in web/src/lib/auto-message.ts.

// AutoMessage builds such a message: kind names it, attrs say what it is
// about (task, repo, pr, path…), context goes to the agent and user is what
// the user wrote, if anything.
func AutoMessage(kind string, attrs map[string]string, context, user string) string {
	keys := make([]string, 0, len(attrs))
	for k, v := range attrs {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(`<dory-context kind="` + html.EscapeString(kind) + `"`)
	for _, k := range keys {
		b.WriteString(" " + k + `="` + html.EscapeString(attrs[k]) + `"`)
	}
	b.WriteString(">\n" + strings.TrimSpace(context) + "\n</dory-context>")
	if user = strings.TrimSpace(user); user != "" {
		b.WriteString("\n\n" + user)
	}
	return b.String()
}
