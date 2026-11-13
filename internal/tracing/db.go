// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// QueryTracer makes one span per query, named by its operation and table, as in
// "SELECT pages". It never records the statement or its values (D6). Off, it is nil.
func QueryTracer() pgx.QueryTracer {
	if !On() {
		return nil
	}
	return queryTracer{}
}

type queryTracer struct{}

func (queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	// Inside a trace that is not kept, the query's span would not be kept either: skip the work
	// (spec 060, load budget).
	if parent := trace.SpanContextFromContext(ctx); parent.IsValid() && !parent.IsSampled() {
		return ctx
	}
	op, table := Statement(d.SQL)
	name := op
	attrs := []attribute.KeyValue{
		attribute.String("db.system.name", "postgresql"),
		attribute.String("db.operation.name", op),
	}
	if table != "" {
		name = op + " " + table
		attrs = append(attrs, attribute.String("db.collection.name", table))
	}
	ctx, span := tracer().Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	return context.WithValue(ctx, querySpan{}, span)
}

// querySpan marks the span a query started, so its end never ends the span around it.
type querySpan struct{}

func (queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	span, ok := ctx.Value(querySpan{}).(trace.Span)
	if !ok {
		return
	}
	if d.Err != nil {
		span.SetStatus(codes.Error, ErrorClass(d.Err))
	}
	span.End()
}

// ErrorClass describes an error by kind only: a database error by its SQLSTATE code, which
// holds no value, and anything else as "error". The message of a database error can quote
// the value that broke a rule, so it never goes into a span (D6).
func ErrorClass(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && len(pg.Code) == 5 {
		return "sqlstate " + pg.Code
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "canceled"
	}
	return "error"
}

// operations are the first words a span may name. Anything else is OTHER.
var operations = map[string]bool{
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true, "WITH": true, "VALUES": true,
	"BEGIN": true, "COMMIT": true, "ROLLBACK": true, "SAVEPOINT": true, "RELEASE": true, "SET": true,
	"LOCK": true, "LISTEN": true, "UNLISTEN": true, "NOTIFY": true, "CREATE": true, "ALTER": true,
	"DROP": true, "TRUNCATE": true, "COPY": true, "CALL": true, "DO": true, "SHOW": true,
	"ANALYZE": true, "VACUUM": true, "REFRESH": true, "GRANT": true, "REVOKE": true, "MERGE": true,
}

// Statement names the operation of a SQL statement and the first table it names after FROM,
// INTO or UPDATE. It reads words and identifiers only, so a literal in the statement can
// never become a name.
func Statement(sql string) (operation, table string) {
	words := strings.FieldsFunc(sql, func(r rune) bool {
		return !(r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	})
	if len(words) == 0 {
		return "OTHER", ""
	}
	operation = strings.ToUpper(words[0])
	if !operations[operation] {
		return "OTHER", ""
	}
	after := map[string]bool{"FROM": true, "INTO": true}
	if operation == "UPDATE" {
		after["UPDATE"] = true
	}
	for i, w := range words[:len(words)-1] {
		if after[strings.ToUpper(w)] && isIdentifier(words[i+1]) {
			return operation, strings.ToLower(words[i+1])
		}
	}
	return operation, ""
}

// isIdentifier accepts a plain SQL name: it starts with a letter or an underscore, is not
// longer than PostgreSQL allows, and is not a keyword that can follow FROM.
func isIdentifier(w string) bool {
	if w == "" || len(w) > 63 || !(w[0] == '_' || w[0] >= 'a' && w[0] <= 'z' || w[0] >= 'A' && w[0] <= 'Z') {
		return false
	}
	switch strings.ToUpper(w) {
	case "SELECT", "LATERAL", "ONLY", "UNNEST", "GENERATE_SERIES", "ROWS", "VALUES":
		return false
	}
	return true
}
