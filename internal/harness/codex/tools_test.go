package codex

import "testing"

func TestCodexCodeModeExec(t *testing.T) {
	tc := codexTool("exec", "const r=await tools.mcp__dory__render_html({html:`<b>x</b>`});")
	if tc.Kind != "mcp" || tc.Server != "dory" || tc.Tool != "render_html" {
		t.Fatalf("tool = %+v", tc)
	}
	out := codexOutput([]any{
		map[string]any{"type": "input_text", "text": "Script completed\nOutput:\n"},
		map[string]any{"type": "input_text", "text": `{"content":[{"type":"text","text":"{\"html_render\":{\"attachment_id\":\"a\"}}"}]}`},
	})
	if out != `{"html_render":{"attachment_id":"a"}}` {
		t.Fatalf("output = %q", out)
	}
}

func TestCodexCodeModeApplyPatchIsEdit(t *testing.T) {
	script := `const patch = "*** Begin Patch\n*** Update File: /r/fib.py\n@@\n def f():\n+    return \"x\"\n*** End Patch";
text(await tools.apply_patch(patch));`
	tc := codexTool("exec", script)
	if tc.Kind != "edit" || tc.Name != "apply_patch" || len(tc.Files) != 1 || tc.Files[0].Path != "/r/fib.py" || tc.Files[0].Added != 1 {
		t.Fatalf("got %+v", tc)
	}
}
func TestCodexTool(t *testing.T) {
	patch := `{"input":"*** Begin Patch\n*** Update File: a.go\n@@\n-old\n+new\n+more\n*** Add File: b.go\n+hi\n*** End Patch"}`
	got := codexTool("apply_patch", patch)
	if got.Kind != "edit" || len(got.Files) != 2 || got.Files[0].Added != 2 || got.Files[0].Removed != 1 || got.Files[1].Change != "add" {
		t.Fatalf("patch: %+v", got.Files)
	}
	cmd := codexTool("shell", `{"command":["bash","-lc","ls -la"]}`)
	if cmd.Kind != "command" || cmd.Command != "ls -la" {
		t.Fatalf("shell: %+v", cmd)
	}
	plan := codexTool("update_plan", `{"plan":[{"step":"a","status":"in_progress"}]}`)
	if plan.Kind != "todo" || plan.Todos[0].Status != "in_progress" {
		t.Fatalf("plan: %+v", plan)
	}
}
