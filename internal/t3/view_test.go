package t3

import (
	"context"
	"testing"
)

func TestThreadViewNormalizesQueueAndActiveRun(t *testing.T) {
	c := rpcServer(t, `{"_tag":"Success","value":{
		"thread":{"title":"T","modelSelection":{"instanceId":"claude","model":"opus","options":[{"id":"effort","value":"high"}]},
			"interactionMode":"plan","runtimeMode":"full-access","branch":"dory/x","worktreePath":"/w",
			"linkedPullRequest":{"repository":"o/r","number":7,"url":"https://github.com/o/r/pull/7"},
			"pullRequests":[{"pullRequest":{"repository":"o/r","number":7,"url":"https://github.com/o/r/pull/7"}}]},
		"runs":[{"id":"r1","status":"running","userMessageId":"m1"},
			{"id":"q2","status":"queued","queuePosition":2,"userMessageId":"m3"},
			{"id":"q1","status":"queued","queuePosition":1,"userMessageId":"m2"}],
		"messages":[{"id":"m1","role":"user","text":"hola"},{"id":"a1","role":"assistant","text":"ok","streaming":true},
			{"id":"m2","role":"user","text":"luego"},{"id":"m3","role":"user","text":"después"}],
		"runtimeRequests":[],"turnItems":[]}}`, nil)
	v, err := c.ThreadView(context.Background(), "th")
	if err != nil {
		t.Fatal(err)
	}
	if v.ActiveRunID != "r1" || v.Effort != "high" || v.EffortOption != "effort" || v.Branch != "dory/x" || v.InteractionMode != "plan" {
		t.Errorf("view %+v", v)
	}
	if len(v.PullRequests) != 1 || v.PullRequests[0].Number != 7 {
		t.Errorf("prs %+v", v.PullRequests)
	}
	if len(v.Queue) != 2 || v.Queue[0].RunID != "q1" || v.Queue[0].Text != "luego" {
		t.Errorf("queue %+v", v.Queue)
	}
	if len(v.Messages) != 2 || !v.Messages[1].Streaming {
		t.Errorf("messages %+v", v.Messages)
	}
}

func TestSendQueuesWithModelAndQueueCommands(t *testing.T) {
	var seen map[string]any
	c := rpcServer(t, `{"_tag":"Success","value":{"sequence":1}}`, &seen)
	err := c.Send(context.Background(), "th", SendInput{Text: "hola", Mode: SendAuto, ProviderID: "codex", Model: "gpt", EffortOption: "reasoningEffort", Effort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	p := seen["payload"].(map[string]any)
	sel := p["modelSelection"].(map[string]any)
	if p["dispatchMode"].(map[string]any)["type"] != "queue_after_active" || sel["model"] != "gpt" || sel["instanceId"] != "codex" {
		t.Errorf("sent %+v", p)
	}
	if err := c.ReorderQueued(context.Background(), "th", "q2", ""); err != nil {
		t.Fatal(err)
	}
	p = seen["payload"].(map[string]any)
	if p["type"] != "queued-run.reorder" || p["beforeRunId"] != nil {
		t.Errorf("reorder %+v", p)
	}
	if err := c.SteerQueued(context.Background(), "th", "q1", "r1"); err != nil {
		t.Fatal(err)
	}
	p = seen["payload"].(map[string]any)
	if p["type"] != "queued-message.promote-to-steer" || p["queuedRunId"] != "q1" || p["targetRunId"] != "r1" {
		t.Errorf("steer %+v", p)
	}
	if c.SetRuntimeMode(context.Background(), "th", "root") == nil {
		t.Error("invalid runtime mode accepted")
	}
}

func TestThreadDiffWithoutCheckpointsIsEmpty(t *testing.T) {
	c := rpcServer(t, `{"_tag":"Success","value":{"checkpoints":[{"appRunOrdinal":null,"status":"ready"},{"appRunOrdinal":3,"status":"missing"}]}}`, nil)
	d, err := c.ThreadDiff(context.Background(), "th")
	if err != nil || d != "" {
		t.Errorf("diff %q err %v", d, err)
	}
}

func TestThreadViewInterleavesReasoningAndTools(t *testing.T) {
	c := rpcServer(t, `{"_tag":"Success","value":{"thread":{},"runs":[],
		"messages":[{"id":"m1","role":"user","text":"hola"},{"id":"a1","role":"assistant","text":"listo"}],
		"turnItems":[
			{"id":"i4","type":"assistant_message","messageId":"a1","ordinal":4},
			{"id":"i1","type":"user_message","messageId":"m1","ordinal":1},
			{"id":"i2","type":"reasoning","text":"pienso","streaming":true,"ordinal":2,"status":"running"},
			{"id":"i3","type":"command_execution","input":"ls -la","ordinal":3,"status":"completed"},
			{"id":"i5","type":"checkpoint","ordinal":5},
			{"id":"i6","type":"run_interrupt_request","ordinal":6},
			{"id":"i7","type":"run_interrupt_result","status":"interrupted","ordinal":7},
			{"id":"i8","type":"proposed_plan","markdown":"# Plan","ordinal":8}]}}`, nil)
	v, err := c.ThreadView(context.Background(), "th")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, m := range v.Messages {
		got = append(got, m.Role+":"+m.Text+m.Detail)
	}
	want := []string{"user:hola", "reasoning:pienso", "tool:ls -la", "assistant:listo", "interrupt:Parado por el usuario", "plan:# Plan"}
	if len(got) != len(want) {
		t.Fatalf("messages %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("messages %v", got)
		}
	}
	if !v.Messages[1].Streaming || v.Messages[2].Title != "command execution" {
		t.Errorf("steps %+v", v.Messages[1:3])
	}
}

func TestThreadViewKeepsPendingQuestions(t *testing.T) {
	c := rpcServer(t, `{"_tag":"Success","value":{"thread":{},"runs":[],"messages":[],
		"runtimeRequests":[{"id":"r1","kind":"user_input","status":"pending"}],
		"turnItems":[{"id":"i1","type":"user_input_request","requestId":"r1","ordinal":1,
			"questions":[{"id":"q","header":"H","question":"¿Color?","multiSelect":true,"options":[{"label":"Rojo","description":"d"}]}]}]}}`, nil)
	v, err := c.ThreadView(context.Background(), "th")
	if err != nil {
		t.Fatal(err)
	}
	if v.Pending == nil || len(v.Pending.Questions) != 1 || !v.Pending.Questions[0].Multi || len(v.Pending.Questions[0].Options) != 1 {
		t.Fatalf("pending %+v", v.Pending)
	}
}

func TestTimelineFoldsAnswersIntoQuestion(t *testing.T) {
	id, text := "async-answer:runtime-request:x", "¿Color? Rojo"
	q := wireTurnItem{ID: "q", Type: "user_input_request", Status: "running", Ordinal: 1}
	q.Questions = append(q.Questions, struct {
		Question string `json:"question"`
	}{"¿Color?"})
	out := timeline([]wireTurnItem{q, {ID: "a", Type: "user_message", MessageID: &id, Text: &text, Ordinal: 2}}, map[string]ViewMessage{}, nil, map[string]bool{}, false)
	if len(out) != 1 || out[0].Title != "Ask User Question" || out[0].Detail != text || out[0].Status != "completed" {
		t.Fatalf("got %+v", out)
	}
}

func TestTimelineShowsCheckpointChanges(t *testing.T) {
	files := []ChangedFile{{Path: "web/a.tsx", Kind: "modified", Additions: 3, Deletions: 1}}
	out := timeline([]wireTurnItem{
		{ID: "c1", Type: "checkpoint", CheckpointID: "cp1", Files: files, Ordinal: 2},
		{ID: "c2", Type: "checkpoint", CheckpointID: "cp2", Ordinal: 3},
	}, map[string]ViewMessage{}, nil, map[string]bool{}, false)
	if len(out) != 1 || out[0].Role != "changes" || out[0].ID != "cp1" || len(out[0].Files) != 1 || out[0].Files[0].Additions != 3 {
		t.Fatalf("got %+v", out)
	}
}
