package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/htmlrender"
	"github.com/migueel15/dory/internal/runtime"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type askInput struct {
	Question  string             `json:"question,omitempty" jsonschema:"A single free question; prefer questions"`
	Questions []runtime.Question `json:"questions,omitempty" jsonschema:"One or more questions, each with a short header and 2-4 options; the user may also answer freely"`
}

type secretInput struct {
	Label       string                `json:"label" jsonschema:"Short title, e.g. sudo password"`
	Reason      string                `json:"reason" jsonschema:"Why you need it, in the user's language"`
	URL         string                `json:"url,omitempty" jsonschema:"Page where the user can get the value"`
	Placeholder string                `json:"placeholder,omitempty"`
	Fields      []runtime.SecretField `json:"fields,omitempty" jsonschema:"1-8 values asked together (e.g. account id + token); omit for a single value"`
	Value       string                `json:"value,omitempty" jsonschema:"A secret the user already pasted in the chat: saved to a file without asking"`
	TimeoutS    int                   `json:"timeout_s,omitempty" jsonschema:"Seconds to wait, default 600, max 3600"`
}

const secretHelp = "Ask the user for private values (passwords, sudo, tokens, API keys, PINs, 2FA codes, account ids) with a secure form; never ask for them in the chat. " +
	"The values never reach you: each is saved to an owner-only file deleted after 24h, and you get its path ({status:saved, file} or files by field name). " +
	"Read a file only inside the command that needs it and never print it, e.g. sudo -S -p '' cmd < \"$file\" or tool --token \"$(cat \"$file\")\". " +
	"Other statuses: declined, timed_out, cancelled, with a note."

type htmlSource struct {
	HTML string `json:"html,omitempty" jsonschema:"Inline page, at most 512000 bytes"`
	Path string `json:"path,omitempty" jsonschema:"Absolute .html file path, instead of html"`
}

type htmlInput struct {
	htmlSource
	Title  string `json:"title" jsonschema:"Short title shown above the page"`
	Height int    `json:"height" jsonschema:"Frame height in CSS pixels, 80 to 2000; use the content_height from html_preview"`
}

type htmlPreviewInput struct {
	htmlSource
	Width int  `json:"width,omitempty" jsonschema:"CSS pixels, 240 to 1600; default 728, about 390 for phones"`
	Light bool `json:"light,omitempty" jsonschema:"Light theme; default dark"`
}

// previewResult is html_preview's answer: the capture and its measures.
type previewResult struct {
	png  []byte
	info string
}

func addUITools(srv *sdk.Server, s *Deps) {
	add(srv, "ask_user", "Ask the user questions with options and wait for the answers in the chat. Use it when a decision is the user's.", func(ctx context.Context, in askInput) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "ask_user only works inside a Dory thread")
		}
		qs := in.Questions
		if in.Question != "" {
			qs = append([]runtime.Question{{Question: in.Question}}, qs...)
		}
		if len(qs) == 0 {
			return nil, apperr.New(apperr.Invalid, "ask_user needs a question")
		}
		answers, err := s.Runtime.Ask(ctx, id, qs)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, len(qs))
		for i, q := range qs {
			var a []string
			if i < len(answers) {
				a = answers[i]
			}
			out[i] = map[string]any{"question": q.Question, "answer": a}
		}
		return map[string]any{"answers": out}, nil
	})

	add(srv, "request_secret", secretHelp, func(ctx context.Context, in secretInput) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "request_secret only works inside a Dory thread")
		}
		if in.Value != "" {
			return s.Runtime.SaveSecret(in.Value)
		}
		p := runtime.SecretPrompt{Label: in.Label, Reason: in.Reason, URL: in.URL, Placeholder: in.Placeholder, Fields: in.Fields}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		timeout := 600 * time.Second
		if in.TimeoutS > 0 {
			timeout = time.Duration(min(in.TimeoutS, 3600)) * time.Second
		}
		return s.Runtime.RequestSecret(ctx, id, p, timeout)
	})

	add(srv, "html_preview", "Check an HTML page before render_html: returns a PNG at the given width, content_height and console output. "+htmlrender.Guide, func(ctx context.Context, in htmlPreviewInput) (any, error) {
		if s.Browser == nil {
			return nil, apperr.New(apperr.Unavailable, "this node has no browser to preview with")
		}
		page, err := htmlrender.Read(in.HTML, in.Path)
		if err != nil {
			return nil, err
		}
		width := htmlrender.ColumnWidth
		if in.Width != 0 {
			width = min(max(in.Width, 240), 1600)
		}
		p, err := s.Browser.Preview(ctx, htmlrender.Document(page, !in.Light), width)
		if err != nil {
			return nil, err
		}
		info, _ := json.Marshal(map[string]any{"content_height": p.ContentHeight, "captured_height": p.CapturedHeight, "width": width, "console": p.Console})
		return previewResult{png: p.PNG, info: string(info)}, nil
	})

	add(srv, "render_html", "Show a finished HTML page (chart, diagram, table, mockup) inline in this thread, above your final reply, in a sandboxed frame. Do not announce or restate the page. Check it first with html_preview. "+htmlrender.Guide, func(ctx context.Context, in htmlInput) (any, error) {
		id, _ := ctx.Value(threadKey{}).(string)
		if id == "" {
			return nil, apperr.New(apperr.Invalid, "render_html only works inside a Dory thread")
		}
		page, err := htmlrender.Read(in.HTML, in.Path)
		if err != nil {
			return nil, err
		}
		title := strings.TrimSpace(in.Title)
		if title == "" || len(title) > htmlrender.MaxTitle {
			return nil, apperr.Newf(apperr.Invalid, "title must have 1 to %d characters", htmlrender.MaxTitle)
		}
		if in.Height < htmlrender.MinHeight || in.Height > htmlrender.MaxHeight {
			return nil, apperr.Newf(apperr.Invalid, "height must be between %d and %d", htmlrender.MinHeight, htmlrender.MaxHeight)
		}
		a, err := s.Runtime.SaveAttachment(id, "page.html", "text/html", strings.NewReader(page))
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": "shown", "html_render": map[string]any{"attachment_id": a.ID, "title": title, "height": in.Height}}, nil
	})
}
