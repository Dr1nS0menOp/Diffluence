// SPDX-License-Identifier: Apache-2.0

package db

import (
	"fmt"
	"regexp"
	"strings"
)

// LintFrom is the first migration the rewrite check reads. The ones before it ran on every
// installation already, or run on the empty tables of a new one.
const LintFrom = 48

// A statement that rewrites a table, or reads all of it while it holds its lock, stops the
// site for as long as that takes (OPS-072, docs/blueprint/07-operations/01).
var (
	createdTable  = regexp.MustCompile(`(?is)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w."]+)`)
	alteredTable  = regexp.MustCompile(`(?is)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?([\w."]+)`)
	indexedTable  = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNIQUE\s+)?INDEX\b(.*?)\bON\s+(?:ONLY\s+)?([\w."]+)`)
	rewriteOK     = regexp.MustCompile(`(?m)^[ \t]*--[ \t]*rewrite-ok:[ \t]*\S`)
	lineComment   = regexp.MustCompile(`--[^\n]*`)
	newType       = regexp.MustCompile(`(?is)\bALTER\s+COLUMN\s+[\w"]+\s+(?:SET\s+DATA\s+)?TYPE\b`)
	setNotNull    = regexp.MustCompile(`(?is)\bSET\s+NOT\s+NULL\b`)
	volatileDflt  = regexp.MustCompile(`(?is)\bADD\s+(?:COLUMN\s+)?.*\bDEFAULT\s+(?:clock_timestamp|random|gen_random_uuid|uuid_generate_v\d\w*|timeofday|nextval)\s*\(`)
	storedColumn  = regexp.MustCompile(`(?is)\bADD\s+(?:COLUMN\s+)?.*\bGENERATED\s+ALWAYS\s+AS\b.*\bSTORED\b`)
	checkedAtOnce = regexp.MustCompile(`(?is)\bADD\s+CONSTRAINT\b.*\b(?:CHECK|FOREIGN\s+KEY)\b`)
	notValid      = regexp.MustCompile(`(?is)\bNOT\s+VALID\b`)
	concurrently  = regexp.MustCompile(`(?is)^\s*CONCURRENTLY\b`)
	vacuumFull    = regexp.MustCompile(`(?is)\bVACUUM\s+(?:\(\s*)?FULL\b`)
	cluster       = regexp.MustCompile(`(?is)\bCLUSTER\b`)
)

// Lint names what a migration does that rewrites a table or holds its lock while it reads
// all of it. A statement may be allowed with a comment "-- rewrite-ok: <reason>" in or right
// above it. Migrations before LintFrom are not read.
func Lint(m Migration) []string {
	if m.Version < LintFrom {
		return nil
	}
	created := map[string]bool{}
	for _, match := range createdTable.FindAllStringSubmatch(lineComment.ReplaceAllString(m.SQL, ""), -1) {
		created[tableKey(match[1])] = true
	}
	var problems []string
	for _, statement := range strings.Split(m.SQL, ";") {
		if rewriteOK.MatchString(statement) {
			continue
		}
		problems = append(problems, lintStatement(lineComment.ReplaceAllString(statement, ""), created)...)
	}
	return problems
}

func lintStatement(code string, created map[string]bool) []string {
	var problems []string
	add := func(table, what string) {
		problems = append(problems, fmt.Sprintf("%s on %s: %s", what, table, strings.Join(strings.Fields(code), " ")))
	}
	if match := alteredTable.FindStringSubmatch(code); match != nil && !created[tableKey(match[1])] {
		table := match[1]
		switch {
		case newType.MatchString(code):
			add(table, "a new column type rewrites the table")
		case setNotNull.MatchString(code):
			add(table, "SET NOT NULL reads the whole table under its lock; add CHECK (... IS NOT NULL) NOT VALID, VALIDATE it, then SET NOT NULL with rewrite-ok")
		case volatileDflt.MatchString(code):
			add(table, "a volatile default rewrites the table; add the column without it and fill it in a background job")
		case storedColumn.MatchString(code):
			add(table, "a stored generated column rewrites the table")
		case checkedAtOnce.MatchString(code) && !notValid.MatchString(code):
			add(table, "a CHECK or FOREIGN KEY reads the whole table under its lock; add it NOT VALID and VALIDATE it in a later migration")
		}
	}
	if match := indexedTable.FindStringSubmatch(code); match != nil && !created[tableKey(match[2])] && !concurrently.MatchString(match[1]) {
		add(match[2], "an index built without CONCURRENTLY blocks writes; build it CONCURRENTLY in a migration with -- transaction: none")
	}
	if vacuumFull.MatchString(code) {
		add("a table", "VACUUM FULL rewrites the table under an exclusive lock")
	} else if cluster.MatchString(code) {
		add("a table", "CLUSTER rewrites the table under an exclusive lock")
	}
	return problems
}

// tableKey makes "public.notes", "notes" and `"notes"` the same table.
func tableKey(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, `"`, ""))
	return strings.TrimPrefix(name, "public.")
}
