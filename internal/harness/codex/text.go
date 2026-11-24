package codex

import (
	"strings"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/kit"
)

const citationOpen = "<oai-mem-citation>"
const citationClose = "</oai-mem-citation>"

type codexText struct {
	*kit.Text
	hidden  bool
	pending string
}

func (s *codexSession) text(id string, kind harness.TextKind) *codexText {
	if s.texts[id] == nil {
		s.texts[id] = &codexText{Text: kit.NewText(s.turn.RunID+":"+id, s.turn.RunID, kind)}
	}
	return s.texts[id]
}

func (s *codexSession) textItem(item codexAppItem, done bool, emit func(harness.Event)) {
	kind, content := harness.Assistant, item.Text
	if item.Type == "reasoning" {
		kind, content = harness.Reasoning, strings.Join(item.Summary, "\n")
	}
	text := s.text(item.ID, kind)
	if !done {
		text.append(content, emit)
		return
	}
	delete(s.texts, item.ID)
	if kind == harness.Assistant {
		content = stripCodexMemoryCitations(content)
	}
	if content != "" || text.String() != "" {
		emit(text.Done(content))
	}
}

func (t *codexText) append(fragment string, emit func(harness.Event)) {
	if t.Kind != harness.Assistant {
		t.Append(fragment, emit)
		return
	}
	input := t.pending + fragment
	var visible strings.Builder
	for {
		tag := citationOpen
		if t.hidden {
			tag = citationClose
		}
		before, after, found := strings.Cut(input, tag)
		t.pending = ""
		if !found {
			for n := min(len(input), len(tag)-1); n > 0; n-- {
				if strings.HasSuffix(input, tag[:n]) {
					t.pending = input[len(input)-n:]
					before = input[:len(input)-n]
					break
				}
			}
		}
		if !t.hidden {
			visible.WriteString(before)
		}
		if !found {
			break
		}
		t.hidden = !t.hidden
		input = after
	}
	t.Append(visible.String(), emit)
}

func stripCodexMemoryCitations(text string) string {
	var visible strings.Builder
	for {
		before, after, found := strings.Cut(text, citationOpen)
		if !found {
			visible.WriteString(text)
			return visible.String()
		}
		_, rest, closed := strings.Cut(after, citationClose)
		if !closed {
			visible.WriteString(text)
			return visible.String()
		}
		visible.WriteString(before)
		text = rest
	}
}
