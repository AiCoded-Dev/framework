package sqltest_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/testmysql"
	"aicoded.dev/framework/sqldb/sqltest"
)

// stopped is a test whose Skip and Fatal record their message and end the goroutine, as a
// test's do.
type stopped struct {
	testing.TB
	how string
}

func (s *stopped) Helper() {}

func (s *stopped) Skip(args ...any) {
	s.how = "skip: " + fmt.Sprint(args...)
	runtime.Goexit()
}

func (s *stopped) Fatal(args ...any) {
	s.how = "fatal: " + fmt.Sprint(args...)
	runtime.Goexit()
}

// open runs sqltest.Open with s and returns how it stopped, or "" when it returned.
func open(s *stopped) string {
	done := make(chan struct{})
	go func() {
		defer close(done)
		sqltest.Open(s)
	}()
	<-done
	return s.how
}

func TestOpenSkipsWithoutADatabase(t *testing.T) {
	t.Setenv(sqltest.EnvDB, "")
	assert.Equal(t, "skip: AICODED_TEST_DB is not set: aicoded check sets it when the app declares sqldb and dev.yaml names a MySQL server",
		open(&stopped{TB: t}))
}

func TestOpenRefusesABadDSN(t *testing.T) {
	t.Setenv(sqltest.EnvDB, "root:s3cret@nonsense(")
	how := open(&stopped{TB: t})
	assert.Equal(t, "fatal: AICODED_TEST_DB does not hold a valid MySQL DSN", how)
	assert.NotContains(t, how, "s3cret")
}

func TestOpen(t *testing.T) {
	cfg := testmysql.Config(t)
	cfg.ParseTime = false
	t.Setenv(sqltest.EnvDB, cfg.FormatDSN())
	db := sqltest.Open(t)
	var now time.Time
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT UTC_TIMESTAMP()").Scan(&now), "times are parsed")
	assert.Equal(t, time.UTC, now.Location())
	var collation string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT @@session.collation_connection").Scan(&collation))
	assert.Equal(t, "utf8mb4_0900_ai_ci", collation)
}
