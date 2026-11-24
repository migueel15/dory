package harness

import "time"

// Message is one entry of a conversation as the interface shows it. Role is
// user, assistant, reasoning, tool, plan or system. An adapter sends the same
// ID again to update a message while it streams.
type Message struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Title     string `json:"title,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Status    string `json:"status,omitempty"`
	Streaming bool   `json:"streaming"`
	CreatedAt string `json:"created_at"`
	// SenderThreadID is the thread whose agent sent this user message.
	SenderThreadID string `json:"sender_thread_id,omitempty"`
	// Tool is set on tool, plan and question messages: the call in Dory's terms.
	Tool *ToolCall `json:"tool,omitempty"`
	// ResultChars is the size of a tool's whole result, before Text clips it.
	// Only the usage ledger reads it.
	InputChars  int `json:"-"`
	ResultChars int `json:"-"`
}

// Window is how much of one usage window of a provider is used, 0 to 1.
type Window struct {
	Kind        string    `json:"kind"`
	Label       string    `json:"label,omitempty"`
	Minutes     int       `json:"window_minutes,omitempty"`
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"`
}

// ReportedSettings contains the settings the harness reports for a turn.
type ReportedSettings struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// LegacyEvent is what an adapter reports while a turn runs. Exactly one of its
// fields is set.
type LegacyEvent struct {
	Message  *Message
	Settings *ReportedSettings
	// Session is the harness id of the conversation, used to resume it.
	Session string
	Usage   []Window
	// Step is the usage of one model call of the turn.
	Step  *StepUsage
	Steps []StepUsage
	// Request pauses the turn until the user answers it.
	Request *Request
	// Done ends the turn; Err is set when it failed.
	Done bool
	Err  string
}

// Turn is one message of the user and everything the agent does after it.
type Turn struct {
	ThreadID    string
	RunID       string
	Dir         string
	Text        string
	Model       string
	Effort      string
	Session     string
	Attachments []Attachment
	// FullAccess skips the harness approval prompts.
	FullAccess bool
	// MCPURL and MCPToken point the agent at the Dory MCP of this node.
	MCPURL, MCPToken string
	// Instructions is session guidance for the model's system prompt. It is
	// never part of Text; see promptWithInstructions for harnesses without a
	// system channel.
	Instructions string
}

// Model is one model a provider offers, with its reasoning effort levels.
type Model struct {
	Slug          string   `json:"slug"`
	Name          string   `json:"name"`
	Efforts       []string `json:"efforts,omitempty"`
	ResolvedModel string   `json:"resolved_model,omitempty"`
}

// Status says whether a harness is installed on this machine.
type Status struct {
	ID string `json:"id"`
	// Driver is the harness: the ID without its account.
	Driver      string `json:"driver,omitempty"`
	DriverName  string `json:"driver_name"`
	ConfigEnv   string `json:"config_env,omitempty"`
	DefaultHome string `json:"default_home,omitempty"`
	Name        string `json:"name"`
	// Home is the configuration directory of the account, when it has its own.
	Home      string `json:"home,omitempty"`
	Installed bool   `json:"installed"`
	// Enabled is false when the provider is turned off on this node.
	Enabled bool    `json:"enabled"`
	Version string  `json:"version"`
	Models  []Model `json:"models"`
	// Catalog says Models came from the harness itself, not the built-in fallback.
	Catalog bool `json:"-"`
	// Capabilities lists what the interface may offer for this provider.
	Capabilities []string `json:"capabilities"`
	LoggedIn     bool     `json:"logged_in"`
	Account      string   `json:"account,omitempty"`
}

// ToolCall is a tool call in Dory's own terms. Each adapter maps the tools of
// its harness onto these kinds, so the chat draws one card per kind whatever
// harness ran it. Only the fields of the kind are set.
type ToolCall struct {
	// Kind is edit, command, todo, plan, read, search, web, agent, mcp, question or other.
	Kind string `json:"kind"`
	// Name is the harness's own name of the tool.
	Name string `json:"name"`

	Files    []FileEdit `json:"files,omitempty"`     // edit
	Command  string     `json:"command,omitempty"`   // command
	ExitCode *int       `json:"exit_code,omitempty"` // command
	Todos    []Todo     `json:"todos,omitempty"`     // todo
	Path     string     `json:"path,omitempty"`      // read, search
	Lines    string     `json:"lines,omitempty"`     // read: "10-40"
	Pattern  string     `json:"pattern,omitempty"`   // search
	URL      string     `json:"url,omitempty"`       // web
	Query    string     `json:"query,omitempty"`     // web
	Agent    string     `json:"agent,omitempty"`     // agent: its type
	Summary  string     `json:"summary,omitempty"`   // agent, command: what it is for, in words
	Steps    int        `json:"steps,omitempty"`     // agent: tool calls it made
	Server   string     `json:"server,omitempty"`    // mcp
	Tool     string     `json:"tool,omitempty"`      // mcp
	Input    string     `json:"input,omitempty"`     // mcp, other: the arguments as JSON

	Questions []Question `json:"questions,omitempty"` // question
}

// Todo is one step of the agent's task list; Status is pending, in_progress or completed.
type Todo struct {
	Text   string `json:"text"`
	Status string `json:"status"`
	// Active is how the step reads while it runs ("Running tests").
	Active string `json:"active,omitempty"`
}

// Question is one question the agent asks the user, with its options.
type Question struct {
	// ID is the harness's own id of the question, when it has one.
	ID          string   `json:"id,omitempty"`
	Question    string   `json:"question"`
	Header      string   `json:"header,omitempty"`
	Options     []Option `json:"options,omitempty"`
	MultiSelect bool     `json:"multi_select,omitempty"`
	// Answer holds the chosen labels or free text once the user answered.
	Answer []string `json:"answer,omitempty"`
}

type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// FileEdit is one file a turn changed; Diff holds unified-diff body lines
// ("+", "-" or " " first).
type FileEdit struct {
	Path    string `json:"path"`
	Change  string `json:"change,omitempty"` // add, update or delete
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Diff    string `json:"diff,omitempty"`
}

// Request is something the agent asks in the middle of a turn; the turn
// waits until the user answers it.
type Request struct {
	ID       string `json:"id"`
	ThreadID string `json:"thread_id,omitempty"`
	// Kind is question or secret; permission will follow.
	Kind string `json:"kind"`
	// MessageID is the tool message that shows the request in the chat.
	MessageID string     `json:"message_id,omitempty"`
	Questions []Question `json:"questions"`
	// Secret is the form of a secret request (kind secret), without values.
	Secret    *SecretPrompt `json:"secret,omitempty"`
	CreatedAt string        `json:"created_at"`
}

// SecretField is one value of a secret form.
type SecretField struct {
	Name        string `json:"name" jsonschema:"Key of the value in the result, e.g. token; letters, digits and _"`
	Label       string `json:"label" jsonschema:"What the user sees above the box"`
	Placeholder string `json:"placeholder,omitempty"`
	// Secret hides what the user types; nil means true.
	Secret *bool `json:"secret,omitempty" jsonschema:"false shows what is typed (an email, an id); default true"`
}

// SecretPrompt is what the user sees: never the values.
type SecretPrompt struct {
	Label       string        `json:"label"`
	Reason      string        `json:"reason"`
	URL         string        `json:"url,omitempty"`
	Placeholder string        `json:"placeholder,omitempty"`
	Fields      []SecretField `json:"fields,omitempty"`
}

// Attachment is a file the user sent with a message. It is saved on the node
// of the thread and the agent reads it from its path.
type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Path string `json:"path"`
}

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

// Entry is a slash command or a skill a harness offers in a folder.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Insert is what the composer writes: how the harness invokes it.
	Insert string `json:"insert"`
}

// External is a conversation a harness saved on this machine, started from
// the harness itself rather than from Dory.
type External struct {
	Provider  string `json:"provider"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Dir       string `json:"dir"`
	Branch    string `json:"branch,omitempty"`
	UpdatedAt string `json:"updated_at"`
	Path      string `json:"-"`
}

// usageEvent is the token usage of one model response read from the local
// logs of a harness.
type UsageEvent struct {
	Time      time.Time
	Provider  string
	Session   string
	Model     string
	Key       string // dedup key; empty when the format needs none
	Tokens    UsageTokens
	Reasoning int64
}

// UsageTokens splits the tokens of a response by how they are billed. Input
// excludes the cached part.
type UsageTokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheWrite int64 `json:"cache_write"`
	CacheRead  int64 `json:"cache_read"`
	// CacheWrite1h is the part of CacheWrite kept for an hour, which costs
	// more than the 5 minute writes (Claude Code writes its cache this way).
	CacheWrite1h int64 `json:"cache_write_1h"`
}
