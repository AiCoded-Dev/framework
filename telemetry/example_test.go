package telemetry_test

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"aicoded.dev/framework/telemetry"
)

var (
	ctx context.Context // the context of a request: it carries the request's span
	db  *sql.DB         // the app's database, from sqldb.Open
)

// Add a span around a step of the app's own that takes time. Its attributes hold ids, counts and
// codes, never personal data.
func ExampleStart() {
	ctx, span := telemetry.Start(ctx, "archive old notes")
	defer span.End()
	res, err := db.ExecContext(ctx, "UPDATE notes SET archived = TRUE WHERE created < ?", time.Now().AddDate(-1, 0, 0))
	if err != nil {
		span.RecordError(err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		span.RecordError(err)
		return
	}
	span.SetAttr("notes", strconv.FormatInt(n, 10))
}
