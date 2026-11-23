package runtime

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
