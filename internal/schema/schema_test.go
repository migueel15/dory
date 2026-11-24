package schema_test

import (
	"strings"
	"testing"

	"github.com/migueel15/dory/internal/localdb"
	"github.com/migueel15/dory/internal/schema"
)

func TestRegistryMatchesMigrations(t *testing.T) {
	db, err := localdb.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, entity := range schema.All() {
		t.Run(entity.Name, func(t *testing.T) {
			rows, err := db.QueryContext(t.Context(), "SELECT name, type, pk FROM pragma_table_info(?)", entity.Table)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			type columnInfo struct {
				kind string
				key  int
			}
			columns := map[string]columnInfo{}
			for rows.Next() {
				var name string
				var col columnInfo
				if err := rows.Scan(&name, &col.kind, &col.key); err != nil {
					t.Fatal(err)
				}
				columns[name] = col
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if len(columns) == 0 {
				t.Fatalf("missing table %s", entity.Table)
			}
			for i, key := range entity.Key {
				if columns[key].key != i+1 {
					t.Errorf("%s.%s is missing or is not primary key column %d", entity.Table, key, i+1)
				}
				delete(columns, key)
			}
			for _, col := range entity.Columns {
				info, ok := columns[col.Name]
				if !ok {
					t.Errorf("missing column %s.%s", entity.Table, col.Name)
				} else if col.Kind == schema.JSON && strings.Contains(strings.ToUpper(info.kind), "INT") {
					t.Errorf("JSON column %s.%s has integer affinity (%s)", entity.Table, col.Name, info.kind)
				}
				delete(columns, col.Name)
			}
			for name := range columns {
				t.Errorf("%s.%s is missing from the registry", entity.Table, name)
			}
		})
	}
}
