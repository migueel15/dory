package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/migueel15/dory/node/importer"
	"github.com/migueel15/dory/node/localstore"
)

// runImport copies the data of a running Dory into a local database, or with
// --check compares per-entity counts against it. It only reads from the source.
func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	from := fs.String("from", "", "URL of the Dory to read from")
	token := fs.String("token", os.Getenv("DORY_TOKEN"), "API token with the read scope (default $DORY_TOKEN)")
	db := fs.String("db", "dory-node.db", "local database file")
	device := fs.String("device", "node", "id of this device")
	check := fs.Bool("check", false, "only compare per-entity counts with the source")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" || *token == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: dory-node import --from URL --token TOKEN [--db FILE] [--device ID] [--check]")
	}
	store, err := localstore.Open(*db, localstore.Options{Device: *device, Local: true})
	if err != nil {
		return err
	}
	defer store.Close()
	ctx := context.Background()
	opts := importer.Options{BaseURL: *from, Token: *token}
	if *check {
		rows, err := importer.Check(ctx, store, opts)
		if err != nil {
			return err
		}
		bad := 0
		for _, r := range rows {
			mark := "ok"
			if !r.OK() {
				mark, bad = "DIFF", bad+1
			}
			fmt.Printf("%-12s source=%-6d local=%-6d %s\n", r.Entity, r.Source, r.Local, mark)
		}
		if bad > 0 {
			return fmt.Errorf("%d entities differ", bad)
		}
		return nil
	}
	rep, err := importer.Run(ctx, store, opts)
	if err != nil {
		return err
	}
	kinds := make([]string, 0, len(rep.Entities))
	for k := range rep.Entities {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		c := rep.Entities[k]
		fmt.Printf("%-12s source=%-6d created=%-6d updated=%-6d unchanged=%d\n", k, c.Source, c.Created, c.Updated, c.Unchanged)
	}
	fmt.Printf("events       written=%d\nchanges=%d ops=%d\n", rep.Events, rep.Changes(), rep.Ops)
	for _, s := range rep.Skipped {
		fmt.Println("skipped:", s)
	}
	return nil
}
