package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"path"
	"strings"
	"time"
)

// StepUsage is what a provider reports for one model call of a turn. Input
// excludes the cached tokens; Reasoning is part of Output. Subagent is set
// when the call belongs to a subagent the turn launched.
type StepUsage struct {
	Model      string
	Input      int64
	CacheRead  int64
	CacheWrite int64
	Output     int64
	Reasoning  int64
	Subagent   string
}

// Usage categories. The provider totals are exact; their split among
// categories is estimated from the size of what each one put in the context.
const (
	catBase      = "base" // unattributed context and tool schemas
	catSystem    = "system"
	catHistory   = "history"
	catUser      = "user"      // the user's message
	catResponse  = "response"  // the agent's text
	catReasoning = "reasoning" // thinking tokens
	catTool      = "tool"      // a tool's call and result; label says which
	catSubagent  = "subagent"  // calls made inside a subagent
)

type ledgerPart struct {
	Category   string
	Label      string
	Input      float64
	CacheRead  float64
	CacheWrite float64
	Output     float64
}

type ledgerItem struct {
	key    [2]string
	tokens float64
}

// ledger follows one turn. Items enter the context in order; each model call
// is split among the items in its context, then its own output joins it.
type ledger struct {
	model       string
	steps       []StepUsage
	total       StepUsage
	parts       map[[2]string]*ledgerPart
	context     map[[2]string]float64
	pending     map[string]*ledgerItem // agent output since the last call, by message id
	order       []string
	written     map[string]bool
	results     map[string]bool // tool messages whose result has been seen
	nextContext []ledgerItem    // results received before the calling step reports usage
}

func newLedger(prompt string, system, history int) *ledger {
	l := &ledger{parts: map[[2]string]*ledgerPart{}, context: map[[2]string]float64{}, pending: map[string]*ledgerItem{}, results: map[string]bool{}, written: map[string]bool{}}
	if prompt != "" {
		l.context[[2]string{catUser, ""}] = estimateTokens(len(prompt))
	}
	for _, item := range []ledgerItem{
		{key: [2]string{catSystem, ""}, tokens: estimateTokens(system)},
		{key: [2]string{catHistory, ""}, tokens: estimateTokens(history)},
	} {
		if item.tokens > 0 {
			l.context[item.key] += item.tokens
		}
	}
	return l
}

// estimateTokens converts characters to tokens with the usual 4:1 ratio.
func estimateTokens(chars int) float64 { return math.Ceil(float64(chars) / 4) }

func (l *ledger) part(key [2]string) *ledgerPart {
	p := l.parts[key]
	if p == nil {
		p = &ledgerPart{Category: key[0], Label: key[1]}
		l.parts[key] = p
	}
	return p
}

// message records what a message adds: the agent's text and tool arguments
// are output of the next call; a tool result is input of the calls after it.
func (l *ledger) message(m Message) {
	switch {
	case m.Role == "reasoning" && m.Text != "":
		l.output(m.ID, [2]string{catReasoning, ""}, len(m.Text))
	case m.Role == "assistant" && m.Text != "":
		l.output(m.ID, [2]string{catResponse, ""}, len(m.Text))
	case m.Tool != nil:
		key := [2]string{catTool, toolLabel(m.Tool)}
		chars := m.InputChars
		if chars == 0 {
			chars = max(len(m.Tool.Input), len(m.Tool.Command)+len(m.Tool.Path)+len(m.Tool.Pattern)+len(m.Tool.Query)+len(m.Tool.URL))
		}
		if !l.written[m.ID] {
			l.output(m.ID, key, chars)
		}
		if (m.Status == "completed" || m.Status == "failed") && !l.results[m.ID] {
			l.results[m.ID] = true
			chars := m.ResultChars
			if chars == 0 {
				chars = len(m.Text)
			}
			result := ledgerItem{key: key, tokens: estimateTokens(chars)}
			if l.written[m.ID] {
				l.context[result.key] += result.tokens
			} else {
				l.nextContext = append(l.nextContext, result)
			}
		}
	}
}

func (l *ledger) output(id string, key [2]string, chars int) {
	if l.written[id] {
		return
	}
	if it := l.pending[id]; it != nil {
		it.tokens = math.Max(it.tokens, estimateTokens(chars))
		return
	}
	l.pending[id] = &ledgerItem{key: key, tokens: estimateTokens(chars)}
	l.order = append(l.order, id)
}

// step splits one model call among the categories.
func (l *ledger) step(s StepUsage) {
	l.steps = append(l.steps, s)
	l.split(s, 1)
}

func (l *ledger) batch(steps []StepUsage) {
	if len(steps) == 0 {
		return
	}
	l.steps = append(l.steps, steps...)
	for _, result := range l.nextContext {
		// The call that issued the tool could not see its result.
		result.tokens *= float64(len(steps)-1) / float64(len(steps))
		l.context[result.key] += result.tokens
	}
	l.nextContext = nil
	// Some providers expose counters only after the turn. Preserve each
	// reported call and apportion their combined usage across the turn.
	var sum StepUsage
	for _, s := range steps {
		sum.Input += s.Input
		sum.Output += s.Output
		sum.CacheRead += s.CacheRead
		sum.CacheWrite += s.CacheWrite
		sum.Reasoning += s.Reasoning
		sum.Model = s.Model
	}
	l.split(sum, len(steps))
}

func (l *ledger) split(s StepUsage, calls int) {
	s.Reasoning = min(s.Reasoning, s.Output)
	l.total.Input += s.Input
	l.total.CacheRead += s.CacheRead
	l.total.CacheWrite += s.CacheWrite
	l.total.Output += s.Output
	l.total.Reasoning += s.Reasoning
	if s.Model != "" && s.Subagent == "" {
		l.model = s.Model
	}
	if s.Subagent != "" {
		p := l.part([2]string{catSubagent, s.Subagent})
		p.Input += float64(s.Input)
		p.CacheRead += float64(s.CacheRead)
		p.CacheWrite += float64(s.CacheWrite)
		p.Output += float64(s.Output)
		return
	}
	// Input: each item in context takes its estimated size; the rest is base.
	processed := float64(s.Input + s.CacheRead + s.CacheWrite)
	if processed > 0 {
		var known float64
		for _, tokens := range l.context {
			known += tokens * float64(calls)
		}
		scale := 1.0
		if known > processed {
			scale = processed / known
		}
		share := func(key [2]string, tokens float64) {
			f := tokens / processed
			p := l.part(key)
			p.Input += f * float64(s.Input)
			p.CacheRead += f * float64(s.CacheRead)
			p.CacheWrite += f * float64(s.CacheWrite)
		}
		for key, tokens := range l.context {
			share(key, tokens*float64(calls)*scale)
		}
		if rest := processed - known*scale; rest > 0 {
			share([2]string{catBase, ""}, rest)
		}
	}
	// Output: reasoning is exact; the rest splits among what the call wrote.
	if s.Reasoning > 0 {
		l.part([2]string{catReasoning, ""}).Output += float64(s.Reasoning)
	}
	visible := float64(s.Output - s.Reasoning)
	var written float64
	for _, id := range l.order {
		if s.Reasoning == 0 || l.pending[id].key[0] != catReasoning {
			written += l.pending[id].tokens
		}
	}
	if visible > 0 {
		if written == 0 {
			l.part([2]string{"unattributed_output", ""}).Output += visible
		}
		for _, id := range l.order {
			if written > 0 {
				it := l.pending[id]
				if s.Reasoning > 0 && it.key[0] == catReasoning {
					continue
				}
				l.part(it.key).Output += visible * it.tokens / written
			}
		}
	}
	for _, id := range l.order {
		l.written[id] = true
		it := l.pending[id]
		if it.key[0] != catReasoning {
			if it.key[0] == catResponse {
				it.key[0] = catHistory
			}
			l.context[it.key] += it.tokens
		}
	}
	for _, it := range l.nextContext {
		l.context[it.key] += it.tokens
	}
	l.nextContext = nil
	l.pending, l.order = map[string]*ledgerItem{}, nil
}

// save stores the turn and its split. A turn without reported usage leaves
// nothing behind.
func (l *ledger) save(ctx context.Context, db *sql.DB, thread, run, provider string, at time.Time) error {
	if len(l.steps) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO usage_turns(run_id, thread_id, provider, model, steps, input, cache_read, cache_write, output, reasoning, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run, thread, provider, l.model, len(l.steps),
		l.total.Input, l.total.CacheRead, l.total.CacheWrite, l.total.Output, l.total.Reasoning, at.UTC().Format(usageTimestamp)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_parts WHERE run_id = ?`, run); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_steps WHERE run_id = ?`, run); err != nil {
		return err
	}
	for index, step := range l.steps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_steps(run_id, ordinal, model, subagent, input, cache_read, cache_write, output, reasoning)
   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, run, index, step.Model, step.Subagent, step.Input, step.CacheRead, step.CacheWrite, step.Output, step.Reasoning); err != nil {
			return err
		}
	}
	for _, p := range l.parts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_parts(run_id, category, label, input, cache_read, cache_write, output) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			run, p.Category, p.Label, p.Input, p.CacheRead, p.CacheWrite, p.Output); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// toolLabel names a tool so that the same tool adds up across providers:
// "mcp:server/tool", "skill:name", or the call's kind (command, read, edit…).
func toolLabel(t *ToolCall) string {
	switch {
	case t.Kind == "mcp" && t.Tool != "":
		return "mcp:" + t.Server + "/" + t.Tool
	case strings.EqualFold(t.Name, "skill"):
		var in struct {
			Skill string `json:"skill"`
		}
		if json.Unmarshal([]byte(t.Input), &in) == nil && in.Skill != "" {
			return "skill:" + in.Skill
		}
		return "skill"
	case strings.EqualFold(path.Base(t.Path), "SKILL.md"):
		return "skill:" + path.Base(path.Dir(t.Path))
	case strings.Contains(t.Command, "SKILL.md"):
		return "skill"
	case t.Kind != "":
		return t.Kind
	}
	return "other"
}

// UsageBreakdownRow groups estimates by thread and category over the selected
// period. The provider totals are conserved before display rounding.
type UsageBreakdownRow struct {
	Thread       string  `json:"thread"`
	Title        string  `json:"title"`
	Provider     string  `json:"provider"`
	Category     string  `json:"category"`
	Label        string  `json:"label"`
	Input        float64 `json:"input"`
	CacheRead    float64 `json:"cache_read"`
	CacheWrite   float64 `json:"cache_write"`
	Output       float64 `json:"output"`
	Cost         float64 `json:"cost"`
	PricedTokens float64 `json:"priced_tokens"`
}

const usageTimestamp = "2006-01-02T15:04:05.000000000Z"

func (m *Manager) UsageBreakdown(ctx context.Context, from time.Time) ([]UsageBreakdownRow, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT t.thread_id, COALESCE(h.title, ''), t.provider, p.category, p.label,
   CASE WHEN EXISTS (SELECT 1 FROM usage_steps s WHERE s.run_id = t.run_id AND s.model != t.model) THEN '' ELSE t.model END AS price_model,
   SUM(p.input), SUM(p.cache_read), SUM(p.cache_write), SUM(p.output)
   FROM usage_turns t JOIN usage_parts p ON p.run_id = t.run_id
 LEFT JOIN threads h ON h.id = t.thread_id
   WHERE t.created_at >= ? GROUP BY t.thread_id, t.provider, p.category, p.label, price_model`, from.UTC().Format(usageTimestamp))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UsageBreakdownRow{}
	for rows.Next() {
		var row UsageBreakdownRow
		var model string
		if err := rows.Scan(&row.Thread, &row.Title, &row.Provider, &row.Category, &row.Label, &model, &row.Input, &row.CacheRead, &row.CacheWrite, &row.Output); err != nil {
			return nil, err
		}
		if _, id, ok := strings.Cut(model, "/"); ok {
			model = id
		}
		if price, ok := priceFor(model); ok {
			row.Cost = (row.Input*price.Input + row.CacheRead*price.CacheRead + row.CacheWrite*price.CacheWrite + row.Output*price.Output) / 1e6
			row.PricedTokens = row.Input + row.CacheRead + row.CacheWrite + row.Output
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// UsageTurn holds the counters recorded for a completed turn.
type UsageTurn struct {
	ID        string          `json:"id"`
	Provider  string          `json:"provider"`
	CreatedAt string          `json:"created_at"`
	Preview   string          `json:"preview"`
	Calls     []UsageTurnCall `json:"calls"`
}

type UsageTurnCall struct {
	UsageTokens
	Ordinal   int    `json:"ordinal"`
	Model     string `json:"model"`
	Reasoning int64  `json:"reasoning"`
	Subagent  bool   `json:"subagent"`
}

func (m *Manager) ThreadUsageTurns(ctx context.Context, thread string, from time.Time) ([]UsageTurn, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT t.run_id, t.provider, t.created_at, COALESCE(substr(msg.text, 1, 120), ''),
			s.ordinal, s.model, s.input, s.cache_read, s.cache_write, s.output, s.reasoning, s.subagent != ''
		FROM usage_turns t JOIN usage_steps s ON s.run_id = t.run_id
		LEFT JOIN thread_messages msg ON msg.id = t.run_id || ':user' AND msg.thread_id = t.thread_id
		WHERE t.thread_id = ? AND t.created_at >= ?
		ORDER BY t.created_at DESC, t.run_id, s.ordinal`, thread, from.UTC().Format(usageTimestamp))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UsageTurn{}
	for rows.Next() {
		var turn UsageTurn
		var call UsageTurnCall
		if err := rows.Scan(&turn.ID, &turn.Provider, &turn.CreatedAt, &turn.Preview,
			&call.Ordinal, &call.Model, &call.Input, &call.CacheRead, &call.CacheWrite,
			&call.Output, &call.Reasoning, &call.Subagent); err != nil {
			return nil, err
		}
		if len(result) == 0 || result[len(result)-1].ID != turn.ID {
			result = append(result, turn)
		}
		current := &result[len(result)-1]
		current.Calls = append(current.Calls, call)
	}
	return result, rows.Err()
}
