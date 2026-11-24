package harness_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/migueel15/dory/internal/harness"
	"github.com/migueel15/dory/internal/harness/procpool"
	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/nodestate"
)

type testDriver struct {
	account harness.Account
	order   int
	pool    *procpool.Pool
}

func (d testDriver) Info() harness.Info {
	return harness.Info{ID: d.account.ID(), Driver: d.account.Driver, Order: d.order, Capabilities: harness.Capabilities{Resume: true}}
}
func (testDriver) Detect(context.Context) harness.Status                        { return harness.Status{} }
func (testDriver) Start(context.Context, harness.Turn) (harness.Session, error) { return nil, nil }
func (d testDriver) WithAccount(a harness.Account) harness.Driver               { d.account = a; return d }
func (d testDriver) WithPool(p *procpool.Pool) harness.Driver                   { d.pool = p; return d }
func (testDriver) ConfigEnv() string                                            { return "TEST_CONFIG" }
func (testDriver) DefaultHome(home string) string                               { return filepath.Join(home, ".test") }
func (d testDriver) AccountHome() string                                        { return d.account.Home }
func (d testDriver) Environ() []string                                          { return []string{"TEST_CONFIG=" + d.account.Home} }

func TestRegistry(t *testing.T) {
	ctx := t.Context()
	db, err := localdb.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	state := nodestate.New(db)
	pool := procpool.New(nil)
	root := t.TempDir()
	late := testDriver{account: harness.Account{Driver: "a"}, order: 2}
	early := testDriver{account: harness.Account{Driver: "z"}, order: 1}
	r := harness.NewRegistry(state, root, pool, late, early)
	a, err := r.AddAccount(ctx, "z", "Trabajo", "")
	if err != nil {
		t.Fatal(err)
	}
	id := "z@trabajo"
	if a.ID() != id || a.Home != filepath.Join(root, "accounts", "z-trabajo") {
		t.Fatalf("unexpected account %+v", a)
	}
	if err := r.SetEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	r = harness.NewRegistry(state, root, pool, late, early)
	var ids []string
	for _, entry := range r.All() {
		ids = append(ids, entry.Info().ID)
	}
	if !reflect.DeepEqual(ids, []string{"z", id, "a"}) {
		t.Fatalf("order: %v", ids)
	}
	d, ok := r.Get(id)
	if !ok || r.Enabled(id) || !r.Capabilities(id).Resume || d.Accounts.AccountHome() != a.Home {
		t.Fatalf("account settings after reload: %+v", d)
	}
	if d.Driver.(testDriver).pool != pool {
		t.Fatal("account did not inherit the injected pool")
	}
	if err := r.SetEnabled(ctx, id, true); err != nil || !r.Enabled(id) {
		t.Fatalf("enable: %v", err)
	}
}
