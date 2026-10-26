package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/testmysql"
)

func TestValidLimits(t *testing.T) {
	list := func(n, size int) []Step {
		out := make([]Step, n)
		for i := range out {
			out[i] = Step{Seq: i + 1, Publish: "p1", Index: i + 1}
		}
		out[0].Statement = strings.Repeat("x", size)
		for i := range out {
			out[i].SHA256 = sha(out[i].Statement)
		}
		return out
	}
	require.NoError(t, valid(list(2000, 1<<20)))
	assert.Equal(t, "E-MIG-004", errs.Code(valid(list(2001, 0))))
	assert.Equal(t, "E-MIG-004", errs.Code(valid(list(1, 1<<20+1))))
}

// A session stopped after a cancelled step is closed, not returned to the pool, and ended in
// the server.
func TestStopDiscardsTheConnection(t *testing.T) {
	pool, other := testmysql.Admin(t), testmysql.Admin(t)
	conn, err := pool.Conn(t.Context())
	require.NoError(t, err)
	s := &session{db: other, conn: conn}
	require.NoError(t, conn.QueryRowContext(t.Context(), "SELECT CONNECTION_ID()").Scan(&s.id))

	s.stop(t.Context())
	assert.Zero(t, pool.Stats().OpenConnections, "the connection did not go back to its pool")
	var n int
	require.NoError(t, other.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID = ?", s.id).Scan(&n))
	assert.Zero(t, n, "the session ended in the server")
}

func TestRunAfterTheContextEnded(t *testing.T) {
	db := testmysql.Admin(t)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = (&session{db: db, conn: conn}).run(ctx, Step{Publish: "p1", Index: 2, Statement: "CREATE TABLE a (x INT)"})
	assert.Equal(t, "E-MIG-005", errs.Code(err))
	require.ErrorContains(t, err, "step p1#2 was not run: context canceled")
}
