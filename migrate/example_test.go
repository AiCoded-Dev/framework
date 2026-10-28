package migrate_test

import (
	"fmt"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/migrate"
)

func ExampleCheck() {
	statements, err := migrate.Check([]byte("CREATE TABLE notes (id BIGINT AUTO_INCREMENT PRIMARY KEY, body TEXT);\n" +
		"ALTER TABLE notes ADD COLUMN author VARCHAR(100) NULL;\n"))
	fmt.Println(len(statements), err)
	// Output: 2 <nil>
}

func ExampleClassify() {
	kind, err := migrate.Classify("ALTER TABLE notes DROP COLUMN author")
	fmt.Println(kind == migrate.Destructive, err)
	_, err = migrate.Classify("DROP DATABASE notes")
	fmt.Println(errs.Code(err))
	// Output:
	// true <nil>
	// E-MIG-002
}
