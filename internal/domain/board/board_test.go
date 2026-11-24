package board

import "testing"

func TestColumn(t *testing.T) {
	cases := []struct {
		name string
		card Card
		want Column
	}{
		{"sin nada va a la bandeja", Card{}, Inbox},
		{"etapa ready va a listas", Card{Stage: "ready"}, Ready},
		{"cancelada por etapa", Card{Stage: "cancelled"}, Cancelled},
		{"pull request cerrada cancela", Card{Stage: "done", HasPRNumber: true, PRState: "closed"}, Cancelled},
		{"pull request mergeada es hecha", Card{HasPRNumber: true, PRState: "merged"}, Done},
		{"hecha a mano sin pull request", Card{Stage: "done"}, Done},
		{"hecha a mano con pull request abierta usa la pull request", Card{Stage: "done", HasPRNumber: true, PRState: "open", Pull: &PullInfo{}}, ReadyToMerge},
		{"borrador trabaja", Card{HasPRNumber: true, PRState: "draft", Pull: &PullInfo{}}, Working},
		{"hilo trabajando", Card{Threads: []string{"working"}}, Working},
		{"hilo esperando necesita al usuario", Card{Threads: []string{"working", "waiting"}}, NeedsYou},
		{"hilo con error necesita al usuario", Card{Threads: []string{"error"}}, NeedsYou},
		{"hilo inactivo cuenta como trabajo", Card{Threads: []string{"idle"}}, Working},
		{"CI fallida necesita al usuario", Card{PRState: "open", Pull: &PullInfo{CI: "FAILURE"}}, NeedsYou},
		{"CI con error necesita al usuario", Card{PRState: "open", Pull: &PullInfo{CI: "ERROR"}}, NeedsYou},
		{"cambios pedidos necesitan al usuario", Card{PRState: "open", Pull: &PullInfo{ReviewDecision: "CHANGES_REQUESTED"}}, NeedsYou},
		{"aprobada lista para mergear", Card{PRState: "open", Pull: &PullInfo{ReviewDecision: "APPROVED", ReviewRequests: []string{"a"}}}, ReadyToMerge},
		{"sin revisión pedida lista para mergear", Card{PRState: "open", Pull: &PullInfo{}}, ReadyToMerge},
		{"revisión pendiente en revisión", Card{PRState: "open", Pull: &PullInfo{ReviewRequests: []string{"a"}}}, InReview},
		{"revisión requerida en revisión", Card{PRState: "open", Pull: &PullInfo{ReviewDecision: "REVIEW_REQUIRED"}}, InReview},
	}
	for _, tc := range cases {
		if got := tc.card.Column(); got != tc.want {
			t.Errorf("%s: columna %s, esperaba %s", tc.name, got, tc.want)
		}
	}
}

func TestFinished(t *testing.T) {
	for name, tc := range map[string]struct {
		card Card
		want bool
	}{
		"cancelada":                      {Card{Stage: "cancelled"}, true},
		"mergeada":                       {Card{PRState: "merged", HasPRNumber: true}, true},
		"hecha a mano":                   {Card{Stage: "done"}, true},
		"hecha con pull request abierta": {Card{Stage: "done", HasPRNumber: true, PRState: "open"}, false},
		"en curso":                       {Card{Stage: "ready"}, false},
	} {
		if got := Finished(tc.card); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}
