package migrate_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/migrate"
)

func TestClassifyDestructive(t *testing.T) {
	for _, stmt := range []string{
		"DROP TABLE t",
		"drop table if exists `t`, u",
		"DROP INDEX i ON t",
		"RENAME TABLE a TO b",
		"rename table a to b, c to d;",
		"TRUNCATE t",
		"TRUNCATE TABLE t",
		"ALTER TABLE t DROP COLUMN c",
		"ALTER TABLE t DROP c",
		"ALTER TABLE t DROP INDEX i, DROP KEY k",
		"ALTER TABLE t DROP FOREIGN KEY f",
		"ALTER TABLE t DROP PRIMARY KEY",
		"ALTER TABLE t DROP CHECK x, DROP CONSTRAINT y",
		"ALTER TABLE t ADD COLUMN c INT, DROP COLUMN d",
		"ALTER TABLE t MODIFY c BIGINT NOT NULL",
		"ALTER TABLE t MODIFY COLUMN c VARCHAR(20) NULL AFTER b",
		"ALTER TABLE t CHANGE c d INT",
		"ALTER TABLE t CHANGE COLUMN c d INT DEFAULT 0",
		"ALTER TABLE t RENAME TO u",
		"ALTER TABLE t RENAME AS u",
		"ALTER TABLE t RENAME COLUMN a TO b",
		"ALTER TABLE t RENAME INDEX a TO b, RENAME KEY c TO d",
		"ALTER TABLE t ALTER COLUMN c SET DEFAULT 1",
		"ALTER TABLE t ALTER c SET DEFAULT ('x')",
		"ALTER TABLE t ALTER c DROP DEFAULT",
		"ALTER TABLE t ALTER COLUMN c SET VISIBLE, ALTER d SET INVISIBLE",
		"ALTER TABLE t ADD INDEX i (c), DROP INDEX j",
	} {
		kind, err := migrate.Classify(stmt)
		require.NoError(t, err, stmt)
		assert.Equal(t, migrate.Destructive, kind, stmt)
	}
}

// What is neither an expand nor a destructive statement on the app's own tables stays refused.
func TestClassifyStillRefused(t *testing.T) {
	for _, stmt := range []string{
		"DROP DATABASE x",
		"DROP SCHEMA x",
		"DROP USER u",
		"DROP VIEW v",
		"DROP TRIGGER t",
		"DROP PROCEDURE p",
		"DROP FUNCTION f",
		"DROP EVENT e",
		"DROP ROLE r",
		"DROP TEMPORARY TABLE t",
		"DROP TABLE t CASCADE",
		"DROP TABLE",
		"DROP TABLE t,",
		"DROP INDEX i ON t ALGORITHM=INPLACE",
		"DROP INDEX IF EXISTS i ON t",
		"DROP INDEX i",
		"TRUNCATE TABLE t, u",
		"TRUNCATE",
		"RENAME TABLE a b",
		"RENAME TABLE a TO b,",
		"RENAME TABLE a AS b",
		"RENAME TABLE a TO b c x TO y",
		"RENAME USER a TO b",
		"DROP TABLE aicoded_migrations",
		"DROP TABLE t, AICODED_MIGRATIONS",
		"RENAME TABLE aicoded_migrations TO x",
		"RENAME TABLE x TO aicoded_migrations",
		"TRUNCATE aicoded_migrations",
		"ALTER TABLE aicoded_migrations DROP COLUMN seq",
		"ALTER TABLE notes RENAME TO other.notes",
		"RENAME TABLE notes TO other.notes",
		"/*!50000 DROP TABLE t */",
		"DROP TABLE db.t",
		"DROP INDEX i ON db.t",
		"ALTER TABLE t ENGINE=InnoDB, DROP INDEX i",
		"ALTER TABLE t DROP COLUMN c, ALGORITHM=INPLACE",
		"ALTER TABLE t ALTER INDEX i INVISIBLE",
		"ALTER TABLE t ALTER CHECK c NOT ENFORCED",
		"ALTER TABLE t ALTER CHECK SET DEFAULT 1",
		"ALTER TABLE t DROP c CASCADE",
		"ALTER TABLE t ALTER COLUMN c SET NOT NULL",
		"ALTER TABLE t ALTER c",
		"ALTER TABLE t DROP PARTITION p1",
		"ALTER TABLE t DROP COLUMN c PARTITION BY HASH(c)",
		"ALTER TABLE t MODIFY c INT PARTITION BY HASH(c)",
		"ALTER TABLE t DROP COLUMN IF EXISTS c",
		"ALTER TABLE t DROP COLUMN a b",
		"ALTER TABLE t DROP FOREIGN f",
		"ALTER TABLE t TRUNCATE PARTITION p",
		"ALTER TABLE t RENAME u",
		"ALTER TABLE t RENAME TO u v",
		"ALTER TABLE t RENAME COLUMN a b",
		"ALTER TABLE t MODIFY c",
		"ALTER TABLE t CHANGE c d",
		"ALTER TABLE t ADD COLUMN c INT DROP COLUMN d",
		"ALTER TABLE t ADD COLUMN modify INT, DROP COLUMN d",
		"ALTER TABLE t DISCARD TABLESPACE",
	} {
		_, err := migrate.Classify(stmt)
		assert.Equal(t, "E-MIG-002", errs.Code(err), stmt)
	}
}

func TestDestructives(t *testing.T) {
	got, err := migrate.Destructives([]string{"CREATE TABLE a (x INT)", "DROP TABLE b", "ALTER TABLE a ADD COLUMN y INT", "ALTER TABLE a DROP x"})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 3}, got)

	got, err = migrate.Destructives([]string{"CREATE TABLE a (x INT)"})
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = migrate.Destructives([]string{"DROP TABLE b", "DROP DATABASE d"})
	assert.Equal(t, "E-MIG-002", errs.Code(err))
}

func TestNotes(t *testing.T) {
	notes := migrate.Notes([]byte("CREATE TABLE a (x INT);\n-- drop it\nDROP TABLE b;\n\nALTER TABLE a\n  DROP COLUMN x;\nINSERT INTO a VALUES (1)"))
	require.Len(t, notes, 2)
	var coded *errs.Error
	require.ErrorAs(t, notes[0], &coded)
	assert.Equal(t, "E-MIG-003", coded.Code)
	assert.Equal(t, ".aicoded/migration.sql:3", coded.Pos)
	assert.Contains(t, coded.Msg, "DROP TABLE")
	assert.Equal(t, "this release needs an approval, and the app stops briefly while the change is applied", coded.Fix)
	require.ErrorAs(t, notes[1], &coded)
	assert.Equal(t, ".aicoded/migration.sql:5", coded.Pos)

	assert.Empty(t, migrate.Notes([]byte("CREATE TABLE a (x INT)")))
	assert.Empty(t, migrate.Notes([]byte("-- nothing")))
}

func TestClassify(t *testing.T) {
	for stmt, code := range map[string]string{
		"CREATE TABLE notes (id BIGINT AUTO_INCREMENT PRIMARY KEY, title VARCHAR(200) NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4": "",
		"create table `n` (a int, b decimal(5,2) default 1.5) comment = 'x.y', collate utf8mb4_bin, auto_increment = 10":               "",
		"CREATE TABLE t (a INT, FOREIGN KEY (a) REFERENCES u (id) ON DELETE CASCADE, CHECK (a LIKE '1%'))":                             "",
		"CREATE INDEX i ON t (a)":                                                       "",
		"create unique index i on t (a, b)":                                             "",
		"CREATE FULLTEXT INDEX i ON t (body)":                                           "",
		"ALTER TABLE t ADD COLUMN c INT":                                                "",
		"ALTER TABLE t ADD COLUMN partition_id INT":                                     "",
		"Alter Table t Add c Int Null":                                                  "",
		"ALTER TABLE t ADD COLUMN c INT NOT NULL DEFAULT 0 AFTER b":                     "",
		"ALTER TABLE t ADD COLUMN c INT DEFAULT -1 COMMENT 'a.b; DROP'":                 "",
		"ALTER TABLE t ADD INDEX i (c), ADD UNIQUE KEY u (d), ADD FULLTEXT KEY f (e)":   "",
		"ALTER TABLE t ADD CONSTRAINT fk FOREIGN KEY (a) REFERENCES u (id)":             "",
		"ALTER TABLE t ADD PRIMARY KEY (id), ADD UNIQUE (b), ADD CONSTRAINT UNIQUE (c)": "",
		"CREATE TABLE t (a INT);":                                                       "",
		"CREATE TABLE t (a INT) COMMENT 'it''s'":                                        "",

		"":                                     "E-MIG-002",
		"/*!50000 DROP TABLE t */":             "E-MIG-002",
		"CREATE TABLE t (a INT) /*+ x */":      "E-MIG-002",
		"CREATE TABLE t (a INT /*!, b INT */)": "E-MIG-002",
		"ALTER TABLE t ADD COLUMN c INT /*!, DROP COLUMN d */":     "E-MIG-002",
		"ALTER TABLE t ADD COLUMN c INT; INSERT INTO u VALUES (1)": "E-MIG-002",
		"ALTER TABLE t ADD FOREIGN KEY (a) REFERENCES d.1e5 (id)":  "E-MIG-002",
		"CREATE TABLE t (a INT) ENGINE=MEMORY":                     "E-MIG-002",
		"ALTER TABLE t ADD CHECK (a > 0)":                          "E-MIG-002",
		"INSERT INTO t VALUES (1)":                                 "E-MIG-002",
		"UPDATE t SET a = 1":                                       "E-MIG-002",
		"DELETE FROM t":                                            "E-MIG-002",
		"REPLACE INTO t VALUES (1)":                                "E-MIG-002",
		"SELECT 1":                                                 "E-MIG-002",
		"SET GLOBAL x = 1":                                         "E-MIG-002",
		"USE other":                                                "E-MIG-002",
		"GRANT ALL ON t TO u":                                      "E-MIG-002",
		"LOCK TABLES t WRITE":                                      "E-MIG-002",
		"DELIMITER //":                                             "E-MIG-002",
		"CREATE VIEW v AS SELECT 1":                                "E-MIG-002",
		"CREATE TRIGGER g BEFORE INSERT ON t FOR EACH ROW SET NEW.a = 1": "E-MIG-002",
		"CREATE PROCEDURE p() BEGIN END":                                 "E-MIG-002",
		"CREATE FUNCTION f() RETURNS INT RETURN 1":                       "E-MIG-002",
		"CREATE EVENT e ON SCHEDULE EVERY 1 DAY DO DELETE FROM t":        "E-MIG-002",

		"CREATE DATABASE d":                    "E-MIG-002",
		"CREATE USER u":                        "E-MIG-002",
		"CREATE TEMPORARY TABLE t (a INT)":     "E-MIG-002",
		"CREATE TABLE IF NOT EXISTS t (a INT)": "E-MIG-002",
		"CREATE TABLE t AS SELECT * FROM u":    "E-MIG-002",
		"CREATE TABLE t LIKE u":                "E-MIG-002",
		"CREATE TABLE t (LIKE u)":              "E-MIG-002",
		"CREATE TABLE t (a INT) SELECT 1":      "E-MIG-002",
		"CREATE TABLE t (a INT) ENGINE=FEDERATED CONNECTION='mysql://h/d/t'": "E-MIG-002",
		"CREATE TABLE t (a INT) PARTITION BY HASH(a)":                        "E-MIG-002",
		"CREATE TABLE t (a INT":                                              "E-MIG-002",
		"CREATE INDEX i ON t (a) ALGORITHM=COPY":                             "E-MIG-002",
		"CREATE TABLE other.t (a INT)":                                       "E-MIG-002",
		"CREATE TABLE other .t (a INT)":                                      "E-MIG-002",
		"ALTER TABLE `other`.`t` ADD COLUMN c INT":                           "E-MIG-002",
		"CREATE INDEX i ON other.t (a)":                                      "E-MIG-002",
		"ALTER TABLE t ADD FOREIGN KEY (a) REFERENCES other.u (id)":          "E-MIG-002",
		"CREATE TABLE t (a INT DEFAULT \"x\")":                               "E-MIG-002",
		"CREATE TABLE t (a VARCHAR(5) DEFAULT 'x\\'')":                       "E-MIG-002",
		"CREATE TABLE t (a INT DEFAULT (SELECT 1))":                          "E-MIG-002",
		"ALTER TABLE t ADD COLUMN c INT DEFAULT (SELECT MAX(a) FROM u)":      "E-MIG-002",
		"CREATE TABLE t (a INT DEFAULT (@x))":                                "E-MIG-002",
		"CREATE TABLE t (a INT); DROP TABLE u":                               "E-MIG-002",
		"CREATE TABLE aicoded_migrations (a INT)":                            "E-MIG-002",
		"ALTER TABLE `AICODED_MIGRATIONS` ADD COLUMN x INT":                  "E-MIG-002",
		"ALTER TABLE aİcoded_mİgratİons ADD COLUMN x INT":                    "E-MIG-002",
		"CREATE TABLE aıcoded_mıgratıons (a INT)":                            "E-MIG-002",
		"CREATE TABLE aicoded_migrationſ (a INT)":                            "E-MIG-002",
		"CREATE TABLE заметки (a INT)":                                       "",
		"ALTER TABLE t ADD COLUMN modify INT":                                "E-MIG-002",
		"ALTER TABLE t ADD COLUMN `modify` INT":                              "",
		"ALTER TABLE t ADD COLUMN c INT NOT NULL":                            "E-MIG-002",
		"ALTER TABLE t ADD c INT PRIMARY KEY":                                "E-MIG-002",
		"ALTER TABLE t ADD (c INT, d INT)":                                   "E-MIG-002",
		"ALTER TABLE t ADD CONSTRAINT c CHECK (a > 0)":                       "E-MIG-002",
		"ALTER TABLE t ADD FULLTEXT (a)":                                     "E-MIG-002",
		"ALTER TABLE t ADD PARTITION (PARTITION p1 VALUES LESS THAN (10))":   "E-MIG-002",
		"ALTER TABLE t ADD COLUMN c INT PARTITION BY HASH(c)":                "E-MIG-002",
		"ALTER TABLE t ENGINE=InnoDB":                                        "E-MIG-002",
		"ALTER TABLE t ADD COLUMN c INT, ALGORITHM=INPLACE":                  "E-MIG-002",
		"ALTER TABLE t ADD COLUMN c INT,":                                    "E-MIG-002",
		"ALTER TABLE t":                                                      "E-MIG-002",
		"ALTER DATABASE d CHARACTER SET utf8mb4":                             "E-MIG-002",
	} {
		kind, err := migrate.Classify(stmt)
		assert.Equal(t, code, errs.Code(err), stmt)
		if code == "" {
			assert.Equal(t, migrate.Expand, kind, stmt)
		}
	}
}

// Each refusal says what to write instead.
func TestClassifyFixes(t *testing.T) {
	for stmt, fix := range map[string]string{
		"CREATE TABLE IF NOT EXISTS t (a INT)":                "drop IF NOT EXISTS: each step runs once",
		"CREATE TEMPORARY TABLE t (a INT)":                    "write CREATE TABLE without TEMPORARY",
		"CREATE TABLE t LIKE u":                               "write CREATE TABLE <name> (<column> <type>, …)",
		"CREATE TABLE t (a INT DEFAULT \"x\")":                "write strings in single quotes",
		"CREATE TABLE t (a VARCHAR(5) DEFAULT 'x\\'')":        "write a quote inside a string as ''",
		"CREATE TABLE t (a INT DEFAULT (@x))":                 "write values as literals",
		"CREATE TABLE t (a INT DEFAULT (SELECT 1))":           "write a DEFAULT as a literal value",
		"CREATE TABLE t (a INT) ENGINE=MEMORY":                "write ENGINE=InnoDB",
		"CREATE TABLE t (a INT) PARTITION BY HASH(a)":         "keep to ENGINE=InnoDB, CHARSET",
		"CREATE TABLE aicoded_migrations (a INT)":             "give your table another name",
		"CREATE TABLE other.t (a INT)":                        "without a database in front",
		"CREATE INDEX i ON t (a) ALGORITHM=COPY":              "leave out USING, ALGORITHM, LOCK",
		"ALTER TABLE t ADD CHECK (a > 0)":                     "check values in the app's code",
		"ALTER TABLE t ADD CONSTRAINT c CHECK (a > 0)":        "check values in the app's code",
		"ALTER TABLE t ADD (c INT, d INT)":                    "write one ADD COLUMN <name> <type> clause per column",
		"ALTER TABLE t ADD COLUMN c INT PARTITION BY HASH(c)": "leave out PARTITION clauses",
		"ALTER TABLE t ADD COLUMN c INT NOT NULL":             "give the column a DEFAULT",
		"ALTER TABLE t ADD COLUMN modify INT":                 "write a column or index named modify in backquotes, as `modify`",
		"/*!50000 DROP TABLE t */":                            "write the statement itself",
	} {
		_, err := migrate.Classify(stmt)
		var coded *errs.Error
		require.ErrorAs(t, err, &coded, stmt)
		assert.Contains(t, coded.Fix, fix, stmt)
	}
}

// Statements written to slip past a naive splitter or classifier are split and classified as
// MySQL reads them.
func TestCheckHidden(t *testing.T) {
	for draft, want := range map[string]struct{ statements, problems []string }{
		"/*!50000 DROP TABLE t */;": {
			[]string{"/*!50000 DROP TABLE t */"}, []string{"1 E-MIG-002"}},
		"CREATE TABLE t (a INT);\n/*! DROP TABLE u */": {
			[]string{"CREATE TABLE t (a INT)", "/*! DROP TABLE u */"}, []string{"2 E-MIG-002"}},
		"CREATE TABLE t (a VARCHAR(30) DEFAULT 'x;DROP TABLE u')": {
			[]string{"CREATE TABLE t (a VARCHAR(30) DEFAULT 'x;DROP TABLE u')"}, nil},
		"INSERT INTO t VALUES ('a;DROP TABLE u')": {
			[]string{"INSERT INTO t VALUES ('a;DROP TABLE u')"}, []string{"1 E-MIG-002"}},
		"CREATE TABLE `t;DROP TABLE u` (a INT)": {
			[]string{"CREATE TABLE `t;DROP TABLE u` (a INT)"}, nil},
		"CREATE TABLE t (a INT) /* ;DROP TABLE u */": {
			[]string{"CREATE TABLE t (a INT)"}, nil},
		"CREATE TABLE t (a INT) -- ;DROP TABLE u\n# ;DROP TABLE v\n;": {
			[]string{"CREATE TABLE t (a INT)"}, nil},
		"ALTER TABLE t ADD COLUMN c INT DEFAULT 1--1;\nDROP DATABASE u": {
			[]string{"ALTER TABLE t ADD COLUMN c INT DEFAULT 1--1", "DROP DATABASE u"}, []string{"2 E-MIG-002"}},
		"\n  CREATE TABLE t (a INT) ;  \n\t\n": {
			[]string{"CREATE TABLE t (a INT)"}, nil},
		"cReAtE tAbLe t (a InT);\ndRoP tAbLe t": {
			[]string{"cReAtE tAbLe t (a InT)", "dRoP tAbLe t"}, nil},
		"ALTER TABLE t\n  ADD COLUMN c INT,\n  DROP COLUMN d;": {
			[]string{"ALTER TABLE t\n  ADD COLUMN c INT,\n  DROP COLUMN d"}, nil},
		"ALTER TABLE t\n  ADD COLUMN c INT,\n  DROP PARTITION d;": {
			[]string{"ALTER TABLE t\n  ADD COLUMN c INT,\n  DROP PARTITION d"}, []string{"1 E-MIG-002"}},
	} {
		statements, err := migrate.Check([]byte(draft))
		assert.Equal(t, want.statements, statements, draft)
		assert.Equal(t, want.problems, problems(err), draft)
	}
}

func TestCheckGathersEveryProblem(t *testing.T) {
	statements, err := migrate.Check([]byte("CREATE TABLE a (x INT);\n-- notes\nDROP USER b;\n\nINSERT INTO a\n  VALUES (1);\nDROP TABLE a"))
	assert.Len(t, statements, 4)
	assert.Equal(t, []string{"3 E-MIG-002", "5 E-MIG-002"}, problems(err), "a destructive statement is not a problem")
	assert.Contains(t, err.Error(), ".aicoded/migration.sql:5: E-MIG-002: ")

	_, err = migrate.Check([]byte("-- only a comment"))
	assert.Equal(t, "E-MIG-001", errs.Code(err))
}

// problems returns the line and code of each coded error that err joins.
func problems(err error) []string {
	if err == nil {
		return nil
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []string{"not joined: " + err.Error()}
	}
	var out []string
	for _, e := range joined.Unwrap() {
		var coded *errs.Error
		if !errors.As(e, &coded) {
			return []string{"not coded: " + e.Error()}
		}
		out = append(out, coded.Pos[strings.LastIndexByte(coded.Pos, ':')+1:]+" "+coded.Code)
	}
	return out
}

func TestCheckPositionsName(t *testing.T) {
	_, err := migrate.Check([]byte("DROP VIEW a"))
	var coded *errs.Error
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, ".aicoded/migration.sql:1", coded.Pos)
}

func TestIsJournal(t *testing.T) {
	for _, name := range []string{"aicoded_migrations", "AICODED_MIGRATIONS", "aİcoded_mİgratİons", "aıcoded_mıgratıons"} {
		assert.True(t, migrate.IsJournal(name), name)
	}
	for _, name := range []string{"aicoded_migrations_log", "app_migrations", "aicoded-migrations", "app.aicoded_migrations", ""} {
		assert.False(t, migrate.IsJournal(name), name)
	}
}
