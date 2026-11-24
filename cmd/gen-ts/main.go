// Command gen-ts generates TypeScript declarations for the JSON API.
package main

//go:generate go run . -out ../../web/src/api/types.gen.ts

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/data"
	"github.com/migueel15/dory/internal/github"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/skills"
	"github.com/migueel15/dory/internal/update"
	"github.com/migueel15/dory/internal/version"
)

const output = "web/src/api/types.gen.ts"

var roots = []any{
	data.Project{}, data.Repository{}, data.RepositoryLocation{}, data.Task{}, data.Dependency{},
	data.Plan{}, data.Memory{}, data.Skill{}, data.SkillState{}, data.Notice{}, data.Setting{},
	data.Thread{}, data.Hit{},
	runtime.Message{}, runtime.ToolCall{}, runtime.FileEdit{}, runtime.Todo{}, runtime.Question{},
	runtime.Option{}, runtime.Request{}, runtime.SecretPrompt{}, runtime.Queued{}, runtime.Attachment{},
	runtime.TurnModel{}, runtime.Window{}, runtime.Model{}, runtime.Status{}, runtime.External{},
	runtime.Job{}, runtime.Delegation{}, runtime.Defaults{}, runtime.KeptWorktree{}, runtime.TitleModel{},
	runtime.Entry{}, runtime.Service{}, runtime.UsageReport{}, runtime.UsageBreakdownRow{}, runtime.UsageTurn{},
	skills.Conflict{}, version.Info{},
	api.ThreadView{}, api.Card{}, api.Settings{}, api.NodeUpdate{}, api.ExternalView{}, peerapi.CreateThreadResponse{},
	github.Pull{}, github.Overview{}, github.Detail{}, github.Closed{}, github.Action{},
	github.DeviceCode{}, github.DeviceStatus{},
}

var names = map[reflect.Type]string{
	reflect.TypeFor[runtime.Service]():     "ThreadService",
	reflect.TypeFor[skills.Conflict]():     "SkillConflict",
	reflect.TypeFor[version.Info]():        "Version",
	reflect.TypeFor[api.Settings]():        "NodeSettings",
	reflect.TypeFor[api.Chord]():           "InputChord",
	reflect.TypeFor[runtime.Request]():     "PendingRequest",
	reflect.TypeFor[runtime.Window]():      "UsageWindow",
	reflect.TypeFor[runtime.Status]():      "Provider",
	reflect.TypeFor[runtime.Defaults]():    "NodeDefaults",
	reflect.TypeFor[update.Status]():       "UpdateStatus",
	reflect.TypeFor[github.DeviceStatus](): "GithubDeviceState",
}

var aliases = map[string]string{
	"Column":       `"inbox" | "ready" | "working" | "needs_you" | "in_review" | "ready_to_merge" | "done" | "cancelled"`,
	"Pull":         "GithubPull",
	"Role":         `"user" | "assistant" | "reasoning" | "tool" | "plan" | "question" | "system" | "interrupt" | "clear" | "model-change" | "changes"`,
	"Stage":        `"inbox" | "ready" | "done" | "cancelled"`,
	"ThreadStatus": `"idle" | "working" | "waiting" | "error"`,
	"ToolKind":     `"edit" | "command" | "todo" | "plan" | "read" | "search" | "web" | "agent" | "mcp" | "question" | "other"`,
}

// String fields whose allowed values are narrower on the wire, plus
// collections with a known nil representation.
var overrides = map[string]string{
	"SkillConflict.kind":               `"link" | "foreign" | "edited"`,
	"UsageReport.bucket":               `"hour" | "day"`,
	"Task.stage":                       "Stage",
	"Thread.status":                    "ThreadStatus",
	"Thread.runtime_mode":              `"full-access" | "approval-required"`,
	"ThreadDefaults.default_workspace": `"checkout" | "worktree" | ""`,
	"Plan.author":                      `"agent" | "user"`,
	"Memory.scope":                     `"global" | "project" | "node"`,
	"Memory.kind":                      `"decision" | "lesson" | "context"`,
	"Hit.entity":                       `"task" | "memory"`,
	"Skill.source":                     `"" | "system"`,
	"SkillState.status":                `"synced" | "pending" | "error" | "conflict" | "removed"`,
	"SkillTargets.nodes":               "string[] | null",
	"SkillTargets.harnesses":           "string[] | null",
	"Message.role":                     "Role",
	"ToolCall.kind":                    "ToolKind",
	"PendingRequest.kind":              `"question" | "permission" | "secret"`,
	"PendingRequest.questions":         "Question[] | null",
	"ThreadView.requests":              "PendingRequest[] | null",
	"Card.column":                      "Column",
	"Card.pr_state":                    `"open" | "draft" | "merged" | "closed"`,
	"Job.kind":                         `"login" | "update" | "install"`,
	"UpdateStatus.state":               `"idle" | "updating" | "failed"`,
	"GithubAction.side":                `"LEFT" | "RIGHT"`,
	"GithubAction.start_side":          `"LEFT" | "RIGHT"`,
	"GithubDeviceState.state":          `"pending" | "done" | "expired" | "denied" | "failed"`,
}

func main() {
	path := flag.String("out", output, "TypeScript output path")
	flag.Parse()
	if err := os.WriteFile(*path, generate(), 0644); err != nil {
		log.Fatal(err)
	}
}

func generate() []byte {
	g := generator{types: map[string]reflect.Type{}, declarations: map[string]string{}}
	for name, expr := range aliases {
		g.declarations[name] = "export type " + name + " = " + expr + ";\n"
	}
	for _, root := range roots {
		g.expression(reflect.TypeOf(root))
	}
	var keys []string
	for name := range g.declarations {
		keys = append(keys, name)
	}
	slices.Sort(keys)
	var out strings.Builder
	out.WriteString("// Code generated by cmd/gen-ts. DO NOT EDIT.\n")
	for _, name := range keys {
		out.WriteByte('\n')
		out.WriteString(g.declarations[name])
	}
	return []byte(out.String())
}

type generator struct {
	types        map[string]reflect.Type
	declarations map[string]string
}

func (g generator) expression(t reflect.Type) string {
	switch t {
	case reflect.TypeFor[json.RawMessage]():
		return "unknown"
	case reflect.TypeFor[time.Time]():
		return "string"
	}
	switch t.Kind() {
	case reflect.Pointer:
		return g.expression(t.Elem()) + " | null"
	case reflect.Slice, reflect.Array:
		item := g.expression(t.Elem())
		if t.Elem().Kind() == reflect.Pointer {
			item = "(" + item + ")"
		}
		return item + "[]"
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return "Record<string, " + g.expression(t.Elem()) + ">"
		}
	case reflect.Interface:
		return "unknown"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Struct:
		if t.Name() == "" {
			return g.object(t)
		}
		return g.declare(t)
	}
	panic(fmt.Sprintf("unsupported JSON type: %v", t))
}

func typeName(t reflect.Type) string {
	if name, ok := names[t]; ok {
		return name
	}
	if t.PkgPath() == "github.com/migueel15/dory/internal/github" {
		return "Github" + t.Name()
	}
	return t.Name()
}

func (g generator) declare(t reflect.Type) string {
	name := typeName(t)
	if previous, ok := g.types[name]; ok {
		if previous != t {
			panic(fmt.Sprintf("TypeScript name %s refers to both %v and %v", name, previous, t))
		}
		return name
	}
	if _, ok := g.declarations[name]; ok {
		panic("TypeScript name conflicts with alias " + name)
	}
	g.types[name] = t
	g.declarations[name] = "export type " + name + " = " + g.object(t) + ";\n"
	return name
}

func (g generator) object(t reflect.Type) string {
	var out strings.Builder
	out.WriteString("{\n")
	fields := map[string]jsonField{}
	var order []string
	for _, field := range jsonFields(t, nil) {
		prev, exists := fields[field.name]
		if !exists {
			order = append(order, field.name)
		}
		// At the shallowest depth, a tagged field beats an untagged field.
		switch {
		case !exists, field.depth < prev.depth, field.depth == prev.depth && field.tagged && !prev.tagged:
			fields[field.name] = field
		case field.depth == prev.depth && field.tagged == prev.tagged:
			prev.ambiguous = true
			fields[field.name] = prev
		}
	}
	for _, name := range order {
		field := fields[name]
		if field.ambiguous {
			continue
		}
		expr, overridden := overrides[typeName(field.owner)+"."+name]
		if !overridden {
			expr = g.expression(field.typ)
		}
		key := strconv.Quote(name)
		if field.optional {
			key += "?"
		}
		fmt.Fprintf(&out, "  %s: %s;\n", key, expr)
	}
	out.WriteByte('}')
	return out.String()
}

type jsonField struct {
	name      string
	owner     reflect.Type
	typ       reflect.Type
	depth     int
	optional  bool
	tagged    bool
	ambiguous bool
}

// jsonFields follows untagged embedded structs and retains field depth for
// encoding/json's name conflict resolution.
func jsonFields(t reflect.Type, parents []reflect.Type) []jsonField {
	if slices.Contains(parents, t) {
		return nil
	}
	var fields []jsonField
	for field := range t.Fields() {
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		embedded := field.Type
		if embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		if field.Anonymous && name == "" && embedded.Kind() == reflect.Struct {
			nested := jsonFields(embedded, append(parents, t))
			for i := range nested {
				nested[i].optional = nested[i].optional || field.Type.Kind() == reflect.Pointer
			}
			fields = append(fields, nested...)
			continue
		}
		if !field.IsExported() {
			continue
		}
		tagged := name != ""
		if !tagged {
			name = field.Name
		}
		optional := slices.Contains(strings.Split(options, ","), "omitempty")
		fields = append(fields, jsonField{name: name, owner: t, typ: field.Type, depth: len(parents), optional: optional, tagged: tagged})
	}
	return fields
}
