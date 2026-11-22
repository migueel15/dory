package runtime

import "strings"

// instructionSection is guidance for one group of Dory tools. It is sent only
// when the thread's MCP catalog offers every tool it names in needs, so the
// model is never told to prefer a tool it does not have.
type instructionSection struct {
	needs []string
	text  string
}

var instructionSections = []instructionSection{
	{[]string{"search", "ensure_thread_task", "save_task", "plan_write"}, `## Tasks and plans
- Dory is the source of truth for tasks, plans and memories; never keep them in files. Read only what the next step needs.
- To work on a task, find an existing one with search, then link or create it with ensure_thread_task. Write its plan with plan_write or plan_edit, never as a plan file.
- Write to Dory only when the work changes state: agreed scope, plan, issue or pull request, cancellation. A write's answer confirms it; do not read it back.
- A task with a pull request is done when it is merged; do not mark it done yourself.`},
	{[]string{"remember", "recall"}, `## Memories
- Before a design decision or when you touch an area with history, recall a few memories by text and project. Do not load memories otherwise.
- Remember only decisions the user agreed and lessons you verified that the code and git history do not tell, with a kind and a short title. Update an outdated memory by id. Use them instead of your own memory files.`},
	{[]string{"subagent_spawn", "subagent_status", "subagent_cancel"}, `## Subagents
- Use subagent_spawn instead of your native subagent tool (Agent, Task, spawn_agent), so the user can follow the child in Dory.
- Do not poll subagent_status, sleep or loop: the child's end wakes you. Cancel a child you no longer need with subagent_cancel.
- As a subagent, do the work yourself and start no subagents unless your task says to.`},
	{[]string{"thread_create", "list_threads", "thread_send"}, `## Threads
- Create a thread with thread_create only when the user asks for a new thread or chat. "Subagent", "delegate" and "in parallel" mean subagent_spawn.
- When the user names another thread, find it with list_threads. Write to it with thread_send.`},
	{[]string{"start_service", "list_services", "stop_service"}, `## Long-running processes
- Start dev servers, databases and watchers with start_service instead of backgrounding them in your shell (&, nohup, setsid, run_in_background). The user sees their ports. Stop them with stop_service.`},
	{[]string{"browser_open", "browser_tabs", "browser_close"}, `## Browser
- browser_* is the browser in the thread's side panel, shared with the user. Use it instead of other browser tools or scripted browsers when the user asks for browser work, names a page, or wants to see a mockup.
- To show a mockup, open its URL with browser_open and check the active tab with browser_tabs.
- Do not use it unasked to verify your own edits. Close tabs you opened before you reply, unless the user asked for them.`},
	{[]string{"device_list", "device_open", "device_screenshot"}, `## Devices
- To drive an Android emulator or device (launch apps, tap, type, swipe, read the screen, screenshots), use the device_* tools so the user can watch it, never adb input, adb shell or scripts that wrap them.
- adb stays fine for builds, installs and logs. If a device tool fails, quote its error and stop instead of falling back to adb.`},
}

// buildInstructions returns the Dory guidance for a session whose MCP catalog
// offers tools. Claude Code defers MCP tools behind ToolSearch, so its copy
// says how to load them.
func buildInstructions(provider, model string, tools []string) string {
	have := make(map[string]bool, len(tools))
	for _, name := range tools {
		have[name] = true
	}
	head := "# Dory\n\nYou run in a Dory thread"
	if provider != "" {
		head += " through " + provider
	}
	if model != "" {
		head += ", as " + strings.Join(strings.Fields(model), " ")
	}
	head += `. The MCP server "dory" lets you manage this thread, its tasks and the user's other threads. When a Dory tool covers what you need, use it instead of your own equivalent: the user follows Dory's tools in its interface.`
	if provider == "claude" {
		head += " Dory tools are deferred: load them with ToolSearch (select:mcp__dory__<name>) before you call them."
	} else {
		head += " If a Dory tool seems missing, look it up in your MCP catalog before you assume it is absent."
	}
	parts := []string{head}
	for _, section := range instructionSections {
		ok := true
		for _, name := range section.needs {
			ok = ok && have[name]
		}
		if ok {
			parts = append(parts, section.text)
		}
	}
	return strings.Join(parts, "\n\n")
}
