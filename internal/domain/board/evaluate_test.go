package board

import "testing"

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name       string
		in         CardInput
		wantCol    Column
		wantReason string
	}{
		{
			name:    "plan accepted with idle thread stays ready",
			in:      CardInput{Source: "internal", Stage: "inbox", PlanAccepted: true, Threads: []string{"idle"}},
			wantCol: Ready,
		},
		{
			name:    "working thread moves to working",
			in:      CardInput{Source: "internal", PlanAccepted: true, Threads: []string{"working"}},
			wantCol: Working,
		},
		{
			name:       "waiting thread moves to needs_you",
			in:         CardInput{Source: "internal", Threads: []string{"waiting"}},
			wantCol:    NeedsYou,
			wantReason: "waiting_thread",
		},
		{
			name:       "idle thread without PR or accepted plan needs you",
			in:         CardInput{Source: "internal", Stage: "inbox", Threads: []string{"idle"}},
			wantCol:    NeedsYou,
			wantReason: "idle_thread",
		},
		{
			name: "1 of 2 PRs merged keeps task active in review",
			in: CardInput{
				Source: "internal",
				Pulls: []PullSummary{
					{Number: 1, State: "merged"},
					{Number: 2, State: "open", ReviewRequests: []string{"alice"}},
				},
			},
			wantCol: InReview,
		},
		{
			name: "2 of 2 PRs merged marks internal task done",
			in: CardInput{
				Source: "internal",
				Pulls: []PullSummary{
					{Number: 1, State: "merged"},
					{Number: 2, State: "merged"},
				},
			},
			wantCol: Done,
		},
		{
			name: "new draft PR on done internal task reopens to working",
			in: CardInput{
				Source: "internal",
				Stage:  "done",
				Pulls: []PullSummary{
					{Number: 1, State: "merged"},
					{Number: 2, State: "draft"},
				},
			},
			wantCol: Working,
		},
		{
			name: "open external issue with all PRs merged needs you",
			in: CardInput{
				Source:        "github",
				ExternalState: "open",
				Pulls: []PullSummary{
					{Number: 10, State: "merged"},
				},
			},
			wantCol:    NeedsYou,
			wantReason: "issue_open_pulls_merged",
		},
		{
			name: "closed external issue NOT_PLANNED is cancelled",
			in: CardInput{
				Source:         "github",
				ExternalState:  "closed",
				ExternalReason: "NOT_PLANNED",
			},
			wantCol:    Cancelled,
			wantReason: "external_not_planned",
		},
		{
			name: "closed external issue COMPLETED is done",
			in: CardInput{
				Source:         "github",
				ExternalState:  "closed",
				ExternalReason: "COMPLETED",
			},
			wantCol:    Done,
			wantReason: "external_closed",
		},
		{
			name: "all PRs closed without merge falls back to inbox",
			in: CardInput{
				Source: "internal",
				Pulls: []PullSummary{
					{Number: 1, State: "closed"},
				},
			},
			wantCol: Inbox,
		},
		{
			name: "CI failure on open PR needs you",
			in: CardInput{
				Source: "internal",
				Pulls: []PullSummary{
					{Number: 1, State: "open", CI: "FAILURE"},
				},
			},
			wantCol:    NeedsYou,
			wantReason: "ci_failed",
		},
		{
			name: "approved open PR is ready to merge",
			in: CardInput{
				Source: "internal",
				Pulls: []PullSummary{
					{Number: 1, State: "open", ReviewDecision: "APPROVED"},
				},
			},
			wantCol: ReadyToMerge,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.in)
			if got.Column != tc.wantCol {
				t.Errorf("Column = %q, want %q", got.Column, tc.wantCol)
			}
			if tc.wantReason != "" && got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}
