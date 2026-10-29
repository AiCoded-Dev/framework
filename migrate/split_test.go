package migrate_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/migrate"
)

func TestSplit(t *testing.T) {
	for draft, want := range map[string][]string{
		"CREATE TABLE a (x INT);\nCREATE TABLE b (y INT)":           {"CREATE TABLE a (x INT)", "CREATE TABLE b (y INT)"},
		"  CREATE TABLE a (x INT) ;  \n\t\n":                        {"CREATE TABLE a (x INT)"},
		"CREATE TABLE a (x INT);;\n;":                               {"CREATE TABLE a (x INT)"},
		"CREATE TABLE a (x VARCHAR(9) DEFAULT 'it''s; ok')":         {"CREATE TABLE a (x VARCHAR(9) DEFAULT 'it''s; ok')"},
		"CREATE TABLE `a;b` (x INT)":                                {"CREATE TABLE `a;b` (x INT)"},
		"CREATE TABLE a (x INT) COMMENT 'a\\';b'":                   {"CREATE TABLE a (x INT) COMMENT 'a\\';b'"},
		"CREATE TABLE a (x INT) /* ; */;\n-- ;\n# ;\n":              {"CREATE TABLE a (x INT)"},
		"ALTER TABLE a ADD/* x */COLUMN b INT":                      {"ALTER TABLE a ADD COLUMN b INT"},
		"/*!50000 DROP TABLE t; */ CREATE TABLE a (x INT)":          {"/*!50000 DROP TABLE t; */ CREATE TABLE a (x INT)"},
		"ALTER TABLE a ADD COLUMN b INT DEFAULT 1--1;\nDROP TABLE":  {"ALTER TABLE a ADD COLUMN b INT DEFAULT 1--1", "DROP TABLE"},
		"ALTER TABLE a ADD COLUMN b INT DEFAULT 1-- 1;\nDROP TABLE": {"ALTER TABLE a ADD COLUMN b INT DEFAULT 1 \nDROP TABLE"},
	} {
		got, err := migrate.Split([]byte(draft))
		require.NoError(t, err, draft)
		assert.Equal(t, want, got, draft)
	}
}

func TestSplitRefuses(t *testing.T) {
	statement := "CREATE TABLE a (x INT) COMMENT '"
	for name, tc := range map[string]struct{ draft, code string }{
		"over 64 KiB":            {statement + strings.Repeat("x", 64<<10-len(statement)) + "'", "E-MIG-001"},
		"not UTF-8":              {"CREATE TABLE a (x INT) COMMENT '\xff'", "E-MIG-001"},
		"no statement":           {" -- nothing\n/* here */ ;", "E-MIG-001"},
		"101 statements":         {strings.Repeat("CREATE TABLE a (x INT);", 101), "E-MIG-001"},
		"a string that goes on":  {"CREATE TABLE a (x INT) COMMENT 'x;\nDROP TABLE b", "E-MIG-002"},
		"a name that goes on":    {"CREATE TABLE `a (x INT)", "E-MIG-002"},
		"a comment that goes on": {"CREATE TABLE a (x INT) /* DROP TABLE b", "E-MIG-002"},
	} {
		_, err := migrate.Split([]byte(tc.draft))
		assert.Equal(t, tc.code, errs.Code(err), name)
	}

	for name, draft := range map[string]string{
		"64 KiB":         statement + strings.Repeat("x", 64<<10-len(statement)-1) + "'",
		"100 statements": strings.Repeat("CREATE TABLE a (x INT);", 100),
	} {
		_, err := migrate.Split([]byte(draft))
		assert.NoError(t, err, name)
	}
}

func TestSplitNamesTheLine(t *testing.T) {
	_, err := migrate.Split([]byte("CREATE TABLE a (x INT);\n\nCREATE TABLE b (x INT) COMMENT 'x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".aicoded/migration.sql:3: E-MIG-002: ")
}

func TestNormalize(t *testing.T) {
	for stmt, want := range map[string]string{
		"CREATE TABLE a (x INT)":                          "CREATE TABLE a ( x INT )",
		"create  table a(\n\tx int\n);":                   "CREATE TABLE a ( x INT )",
		"CREATE /* new */ TABLE a (x INT) -- notes":       "CREATE TABLE a ( x INT )",
		"CREATE TABLE Notes (x INT)":                      "CREATE TABLE Notes ( x INT )",
		"create index i on Notes (x)":                     "CREATE INDEX i ON Notes ( x )",
		"alter table Notes add column y int default 0x1f": "ALTER TABLE Notes ADD COLUMN y INT DEFAULT 0x1f",
		"ALTER TABLE n ADD FOREIGN KEY (x) REFERENCES Owners (id) on delete cascade": "ALTER TABLE n ADD FOREIGN KEY ( x ) " +
			"REFERENCES Owners ( id ) ON DELETE CASCADE",
		"CREATE TABLE a (x INT) COMMENT 'Two  spaces'": "CREATE TABLE a ( x INT ) COMMENT 'Two  spaces'",
		"CREATE TABLE `My  Notes` (`Body` TEXT)":       "CREATE TABLE `My  Notes` ( `Body` TEXT )",
		"  CREATE TABLE a (x INT) COMMENT 'it''s'  ":   "CREATE TABLE a ( x INT ) COMMENT 'it''s'",
		"CREATE TABLE a (x INT) COMMENT 'goes on":      "CREATE TABLE a (x INT) COMMENT 'goes on",
	} {
		assert.Equal(t, want, migrate.Normalize(stmt), stmt)
	}
}

func TestNormalizeKeepsNames(t *testing.T) {
	assert.NotEqual(t, migrate.Normalize("insert into Visitors values (1)"), migrate.Normalize("INSERT INTO visitors VALUES (1)"))
	assert.Equal(t, migrate.Normalize("Create Table Visitors (Id Int Not Null)"), migrate.Normalize("CREATE TABLE Visitors (Id INT NOT NULL)"))
	for stmt, want := range map[string]string{
		"drop table if exists Notes":            "DROP TABLE IF EXISTS Notes",
		"create table if not exists T (id int)": "CREATE TABLE IF NOT EXISTS T ( id INT )",
		"alter table a rename to B":             "ALTER TABLE a RENAME TO B",
		"select 1":                              "SELECT 1",
		"alter table a modify column Total decimal(10, 2) unsigned not null default 0": "ALTER TABLE a MODIFY COLUMN Total " +
			"DECIMAL ( 10 , 2 ) UNSIGNED NOT NULL DEFAULT 0",
		"create table a (At datetime default current_timestamp) engine=innodb default charset=utf8mb4": "CREATE TABLE a ( At DATETIME " +
			"DEFAULT CURRENT_TIMESTAMP ) ENGINE = INNODB DEFAULT CHARSET = utf8mb4",
	} {
		assert.Equal(t, want, migrate.Normalize(stmt), stmt)
	}
}
