// Package testutil provides isolated databases for retrieval tests.
package testutil

import (
	"context"
	"fmt"
	"testing"

	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
	_ "github.com/mattn/go-sqlite3"
	"sev0/ent"
	"sev0/ent/migrate"
	_ "sev0/ent/runtime"
)

func Database(t *testing.T) *ent.Client       { return database(t, false) }
func LegacyDatabase(t *testing.T) *ent.Client { return database(t, true) }
func database(t *testing.T, legacy bool) *ent.Client {
	t.Helper()
	c, err := ent.Open("sqlite3", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	tables := testTables(t)
	if legacy {
		added := map[string]bool{"guild_id": true, "channel_id": true, "reply_to_id": true}
		for _, table := range tables {
			if table.Name == "discord_messages" {
				var columns []*schema.Column
				for _, column := range table.Columns {
					if !added[column.Name] {
						columns = append(columns, column)
					}
				}
				table.Columns = columns
				var indexes []*schema.Index
				for _, index := range table.Indexes {
					keep := true
					for _, column := range index.Columns {
						if added[column.Name] {
							keep = false
						}
					}
					if keep {
						indexes = append(indexes, index)
					}
				}
				table.Indexes = indexes
			}
		}
	}
	if err = migrate.Create(context.Background(), c.Schema, tables); err != nil {
		t.Fatal(err)
	}
	return c
}
func Migrate(t *testing.T, c *ent.Client) {
	t.Helper()
	if err := migrate.Create(context.Background(), c.Schema, testTables(t)); err != nil {
		t.Fatal(err)
	}
}
func testTables(t *testing.T) []*schema.Table {
	t.Helper()
	tables, err := schema.CopyTables(migrate.Tables)
	if err != nil {
		t.Fatal(err)
	}
	// SQLite has no pgvector type. Only the unused embedding column is adapted;
	// retrieval runs against the generated message/user schema and real SQL.
	for _, table := range tables {
		for _, column := range table.Columns {
			if column.Name == "embedding" {
				column.Type = field.TypeString
				column.SchemaType = map[string]string{"sqlite3": "text"}
			}
		}
	}
	return tables
}
