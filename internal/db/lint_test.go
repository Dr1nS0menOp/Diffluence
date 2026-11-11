// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/db"
)

func lint(sql string) []string {
	return db.Lint(db.Migration{Version: db.LintFrom, Name: "x", Phase: db.PhaseExpand, SQL: "-- phase: expand\n" + sql})
}

// OPS-072: statements that rewrite a table or hold its lock while they read all of it.
func TestStatementsThatRewriteOrLongLockATableAreRefused(t *testing.T) {
	refused := map[string]string{
		"a new type":                 "ALTER TABLE pages ALTER COLUMN title TYPE varchar(300);",
		"a new type, long form":      "ALTER TABLE pages ALTER COLUMN title SET DATA TYPE varchar(300);",
		"not null at once":           "ALTER TABLE pages ALTER COLUMN title SET NOT NULL;",
		"a volatile default":         "ALTER TABLE pages ADD COLUMN token uuid DEFAULT gen_random_uuid();",
		"a clock default":            "ALTER TABLE pages ADD COLUMN seen timestamptz NOT NULL DEFAULT clock_timestamp();",
		"a stored column":            "ALTER TABLE pages ADD COLUMN lower_title text GENERATED ALWAYS AS (lower(title)) STORED;",
		"an index that blocks":       "CREATE INDEX pages_title ON pages (title);",
		"a unique index that blocks": "CREATE UNIQUE INDEX IF NOT EXISTS pages_slug ON pages (slug);",
		"a check at once":            "ALTER TABLE pages ADD CONSTRAINT short CHECK (length(title) < 300);",
		"a foreign key at once":      "ALTER TABLE pages ADD CONSTRAINT owner FOREIGN KEY (owner_id) REFERENCES users (id);",
		"vacuum full":                "VACUUM FULL pages;",
		"cluster":                    "CLUSTER pages USING pages_pkey;",
		"lower case":                 "alter table pages alter column title type text;",
	}
	for name, sql := range refused {
		if problems := lint(sql); len(problems) == 0 {
			t.Errorf("%s: passed: %s", name, sql)
		}
	}
}

func TestSafeChangesPass(t *testing.T) {
	allowed := map[string]string{
		"a column with a fixed default": "ALTER TABLE pages ADD COLUMN pinned boolean NOT NULL DEFAULT false;",
		"a column with now()":           "ALTER TABLE pages ADD COLUMN seen timestamptz NOT NULL DEFAULT now();",
		"an index built alongside":      "CREATE INDEX CONCURRENTLY IF NOT EXISTS pages_title ON pages (title);",
		"a check in two steps":          "ALTER TABLE pages ADD CONSTRAINT short CHECK (length(title) < 300) NOT VALID;",
		"a new table and its index":     "CREATE TABLE notes (id uuid PRIMARY KEY, body text NOT NULL);\nCREATE INDEX notes_body ON notes (body);\nALTER TABLE notes ALTER COLUMN body TYPE varchar(10);",
		"a new table, quoted name":      "CREATE TABLE IF NOT EXISTS public.notes (id int);\nCREATE INDEX notes_id ON public.notes (id);",
		"dropping is a contract matter": "ALTER TABLE pages DROP COLUMN old_title;",
		"a word in a comment":           "-- We never CLUSTER or VACUUM FULL here.\nSELECT 1;",
	}
	for name, sql := range allowed {
		if problems := lint(sql); len(problems) > 0 {
			t.Errorf("%s: refused %v", name, problems)
		}
	}
}

// An exception is written on the statement, with the reason, as for the scanners.
func TestAnExceptionNeedsAReason(t *testing.T) {
	with := "-- rewrite-ok: the table holds one row per instance\nALTER TABLE instance ALTER COLUMN name TYPE text;"
	if problems := lint(with); len(problems) > 0 {
		t.Fatalf("an exception with a reason was refused: %v", problems)
	}
	without := "-- rewrite-ok:\nALTER TABLE instance ALTER COLUMN name TYPE text;"
	if problems := lint(without); len(problems) == 0 {
		t.Fatal("an exception without a reason passed")
	}
	// The exception covers its own statement only.
	two := with + "\nALTER TABLE pages ALTER COLUMN title TYPE text;"
	if problems := lint(two); len(problems) != 1 || !strings.Contains(problems[0], "pages") {
		t.Fatalf("the exception spread to the next statement: %v", problems)
	}
}

func TestEarlierMigrationsAreNotChecked(t *testing.T) {
	old := db.Migration{Version: db.LintFrom - 1, Phase: db.PhaseExpand, SQL: "-- phase: expand\nCREATE INDEX a ON pages (title);"}
	if problems := db.Lint(old); len(problems) > 0 {
		t.Fatalf("a migration from before the rule was checked: %v", problems)
	}
}

// The rule itself: every migration of this release from LintFrom on passes it.
func TestNoMigrationRewritesATable(t *testing.T) {
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		for _, p := range db.Lint(m) {
			t.Errorf("%04d_%s: %s", m.Version, m.Name, p)
		}
	}
}
