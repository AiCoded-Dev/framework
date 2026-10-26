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

		"DROP TABLE t":                "E-MIG-003",
		"drop index i on t":           "E-MIG-003",
		"RENAME TABLE a TO b":         "E-MIG-003",
		"TRUNCATE t":                  "E-MIG-003",
		"ALTER TABLE t DROP COLUMN c": "E-MIG-003",
		"ALTER TABLE t ADD COLUMN c INT, DROP COLUMN d": "E-MIG-003",
		"ALTER TABLE t ENGINE=InnoDB, DROP INDEX i":     "E-MIG-003",
		"ALTER TABLE t MODIFY c BIGINT":                 "E-MIG-003",
		"ALTER TABLE t CHANGE c d INT":                  "E-MIG-003",
		"ALTER TABLE t RENAME TO u":                     "E-MIG-003",
		"ALTER TABLE t RENAME COLUMN a TO b":            "E-MIG-003",
		"ALTER TABLE t ALTER COLUMN c SET DEFAULT 1":    "E-MIG-003",
		"ALTER TABLE t ALTER INDEX i INVISIBLE":         "E-MIG-003",

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
		"CREATE TRIGGER g BEFORE INSERT ON t FOR EACH ROW SET NEW.a = 1":     "E-MIG-002",
		"CREATE PROCEDURE p() BEGIN END":                                     "E-MIG-002",
		"CREATE FUNCTION f() RETURNS INT RETURN 1":                           "E-MIG-002",
		"CREATE EVENT e ON SCHEDULE EVERY 1 DAY DO DELETE FROM t":            "E-MIG-002",
		"CREATE DATABASE d":                                                  "E-MIG-002",
		"CREATE USER u":                                                      "E-MIG-002",
		"CREATE TEMPORARY TABLE t (a INT)":                                   "E-MIG-002",
		"CREATE TABLE IF NOT EXISTS t (a INT)":                               "E-MIG-002",
		"CREATE TABLE t AS SELECT * FROM u":                                  "E-MIG-002",
		"CREATE TABLE t LIKE u":                                              "E-MIG-002",
		"CREATE TABLE t (LIKE u)":                                            "E-MIG-002",
		"CREATE TABLE t (a INT) SELECT 1":                                    "E-MIG-002",
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
		"ALTER TABLE t ADD COLUMN c INT DEFAULT 1--1;\nDROP TABLE u": {
			[]string{"ALTER TABLE t ADD COLUMN c INT DEFAULT 1--1", "DROP TABLE u"}, []string{"2 E-MIG-003"}},
		"\n  CREATE TABLE t (a INT) ;  \n\t\n": {
			[]string{"CREATE TABLE t (a INT)"}, nil},
		"cReAtE tAbLe t (a InT);\ndRoP tAbLe t": {
			[]string{"cReAtE tAbLe t (a InT)", "dRoP tAbLe t"}, []string{"2 E-MIG-003"}},
		"ALTER TABLE t\n  ADD COLUMN c INT,\n  DROP COLUMN d;": {
			[]string{"ALTER TABLE t\n  ADD COLUMN c INT,\n  DROP COLUMN d"}, []string{"1 E-MIG-003"}},
	} {
		statements, err := migrate.Check([]byte(draft))
		assert.Equal(t, want.statements, statements, draft)
		assert.Equal(t, want.problems, problems(err), draft)
	}
}

func TestCheckGathersEveryProblem(t *testing.T) {
	statements, err := migrate.Check([]byte("CREATE TABLE a (x INT);\n-- notes\nDROP TABLE b;\n\nINSERT INTO a\n  VALUES (1);\nCREATE INDEX i ON a (x)"))
	assert.Len(t, statements, 4)
	assert.Equal(t, []string{"3 E-MIG-003", "5 E-MIG-002"}, problems(err))
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
	_, err := migrate.Check([]byte("DROP TABLE a"))
	var coded *errs.Error
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, ".aicoded/migration.sql:1", coded.Pos)
}
