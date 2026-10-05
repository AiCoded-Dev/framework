package lint

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteOf(t *testing.T) {
	for sql, want := range map[string]Write{
		"INSERT INTO notes (body) VALUES (?)":                                                              {Table: "notes", Op: "INSERT"},
		"insert low_priority ignore into `app`.`notes`(body) values (?)":                                   {Table: "app.notes", Op: "INSERT"},
		"-- c\n# c\n/* c */ UPDATE IGNORE notes n JOIN tags t ON t.id = n.tag SET n.body = ?":              {Table: "notes", Op: "UPDATE"},
		"DELETE QUICK FROM notes WHERE id = ?":                                                             {Table: "notes", Op: "DELETE"},
		"DELETE n, t FROM notes n JOIN tags t ON t.id = n.tag":                                             {Table: "notes", Op: "DELETE"},
		"REPLACE tags (name) VALUES (?)":                                                                   {Table: "tags", Op: "REPLACE"},
		"WITH n AS (SELECT 1) UPDATE notes SET body = ''":                                                  {Table: "notes", Op: "UPDATE"},
		"with recursive a (x) as (select ')' union all select x from a), b as (select 1) delete from tags": {Table: "tags", Op: "DELETE"},
		"WITH a AS (SELECT 1--1) DELETE FROM notes":                                                        {Table: "notes", Op: "DELETE"},
		"WITH `a``)` AS (SELECT 1) DELETE FROM notes":                                                      {Table: "notes", Op: "DELETE"},
		"DELETE FROM notes -- UPDATE tags\n WHERE id = 1":                                                  {Table: "notes", Op: "DELETE"},
		"DELETE " + strings.Repeat("n, ", 100) + "t FROM notes n JOIN tags t ON t.id = n.tag":              {Table: "notes", Op: "DELETE"},
		"INSERT /*+ SET_VAR(foreign_key_checks = OFF) */ INTO notes VALUES (?)":                            {Table: "notes", Op: "INSERT"},
	} {
		got, ok := writeOf(sql)
		assert.True(t, ok, sql)
		assert.Equal(t, want, got, sql)
	}
	for _, sql := range []string{
		"SELECT * FROM notes",
		"SELECT 1 -- DELETE FROM notes",
		"WITH n AS (DELETE FROM notes) SELECT 1",
		"WITH n AS (SELECT 'UPDATE notes') SELECT 1",
		"INSERT INTO (",
		"DELETE notes",
		"",
	} {
		_, ok := writeOf(sql)
		assert.False(t, ok, sql)
	}
}
