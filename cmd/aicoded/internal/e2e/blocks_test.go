package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/testmysql"
)

var (
	blocksOnce sync.Once
	blocks     *site
	blocksErr  error
)

// startBlocks starts the blocks app once per test binary, on the MySQL server of
// AICODED_TEST_MYSQL_DSN, after dropping what a killed run may have left there.
func startBlocks(t *testing.T) *site {
	t.Helper()
	requireGo(t)
	dsn := testmysql.DSN(t)
	blocksOnce.Do(func() {
		dropDatabases(dsn, "blocks")
		blocks, blocksErr = launchApp("blocks", dsn)
		if blocksErr != nil {
			dropDatabases(dsn, "blocks")
			return
		}
		blocks.onStop = func() { dropDatabases(dsn, "blocks") }
	})
	require.NoError(t, blocksErr)
	return blocks
}

// dropDatabases drops the databases and the users the dev runner made for apps.
func dropDatabases(dsn string, apps ...string) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return
	}
	c, err := mysql.NewConnector(cfg)
	if err != nil {
		return
	}
	db := sql.OpenDB(c)
	defer db.Close()
	for _, app := range apps {
		for _, stmt := range []string{"DROP USER IF EXISTS 'ac-dev-" + app + "'@'%'", "DROP DATABASE IF EXISTS `ac-dev-" + app + "`"} {
			if _, err := db.ExecContext(context.Background(), stmt); err != nil {
				fmt.Fprintf(os.Stderr, "e2e: %s: %v\n", stmt, err)
			}
		}
	}
}

func TestBlocksAddItem(t *testing.T) {
	s := startBlocks(t)
	_, page := s.get(t, "editor", "/items")
	resp, _ := s.post(t, "editor", "/items", url.Values{
		"_aicoded_form": {field(t, page, "_aicoded_form")},
		"_aicoded_csrf": {field(t, page, "_aicoded_csrf")},
		"title":         {"Fix the <boiler>"},
	})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	item := resp.Header.Get("Location")

	resp, body := s.get(t, "editor", item)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, `<h1 id="title">Fix the &lt;boiler&gt;</h1>`, "the row came back from MySQL")
	assert.Contains(t, body, `<pre id="doc">Fix the &lt;boiler&gt;</pre>`, "the document came back from the store")

	var rows int
	require.NoError(t, testmysql.Admin(t).QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM `ac-dev-blocks`.items WHERE title = ?", "Fix the <boiler>").Scan(&rows))
	assert.Positive(t, rows, "the row is in the app's own database")

	sent, err := s.ws.Mail(t.Context(), s.app)
	require.NoError(t, err)
	require.NotEmpty(t, sent)
	require.Equal(t, "sent", sent[0].Folder)
	last, err := s.ws.MailGet(t.Context(), s.app, sent[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "app@blocks.test", last.From)
	require.NotEmpty(t, last.To)
	assert.Equal(t, "team@blocks.test", last.To[0])
	assert.Equal(t, "Fix the <boiler>", last.Text)
}

func TestBlocksInbox(t *testing.T) {
	s := startBlocks(t)
	_, err := s.ws.MailReceive(t.Context(), s.app, devapi.InboundMail{From: "ana@blocks.test", Subject: "Hello from outside", Text: "hi"})
	require.NoError(t, err)
	_, body := s.get(t, "editor", "/inbox")
	assert.Contains(t, body, "Hello from outside")
}
