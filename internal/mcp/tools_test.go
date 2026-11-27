package mcp

import (
	"slices"
	"strings"
	"testing"
)

// The tools are a contract with the agents and the dory-workflow skill: the
// list changes only together with them.
func TestToolNames(t *testing.T) {
	want := strings.Split("add_task_dependency,ask_user,browser_activate,browser_click,browser_close,browser_console,browser_key,browser_navigate,browser_open,browser_read,browser_resize,browser_screenshot,browser_scroll,browser_tabs,browser_type,comment_task,create_pull,ensure_thread_task,get_pull,get_task,get_thread,html_preview,link_pull,link_thread,list_environments,list_models,list_projects,list_pulls,list_services,list_task_threads,list_tasks,list_threads,merge_pull,plan_edit,plan_read,plan_write,recall,remember,remove_task_dependency,render_html,request_secret,review_pull,save_task,search,session_start,set_task_project,set_task_state,start_service,stop_service,subagent_cancel,subagent_spawn,subagent_status,terminal_close,terminal_list,terminal_open,terminal_read,terminal_rename,terminal_run,terminal_write,thread_configure,thread_create,thread_send,thread_stop,unlink_pull,unlink_thread,update_pull", ",")
	var got []string
	for name := range toolsOf(t, build(&Deps{}, false, true)) {
		got = append(got, name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("tools changed:\n got %v\nwant %v", got, want)
	}
}
