// Package draft reads and checks an app's schema-change draft, .aicoded/migration.sql, for
// aicoded check.
package draft

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/migrate"
)

// File is where an app keeps its draft, relative to its folder.
const File = ".aicoded/migration.sql"

// maxSize is the most bytes a draft may hold; Read reads no more than one byte past it.
const maxSize = 64 << 10

// Read returns the draft of the app in dir, or nil when the app has none. It reads the draft
// only inside dir, and refuses with E-MIG-001 one that is not a regular file, lies outside dir,
// cannot be read or holds more than 64 KiB.
func Read(dir string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Stat(File)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, unreadable(err)
	case !info.Mode().IsRegular():
		return nil, errs.At(File, "E-MIG-001", "the draft is not a regular file", "make "+File+" a plain file of SQL statements")
	case info.Size() > maxSize:
		return nil, tooLarge(info.Size())
	}
	f, err := root.Open(File)
	if err != nil {
		return nil, unreadable(err)
	}
	defer f.Close()
	text, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, unreadable(err)
	}
	if len(text) > maxSize {
		return nil, tooLarge(int64(len(text)))
	}
	return text, nil
}

// Check reads the draft of the app in dir, whose permission list is m, and checks it with
// migrate.Check. It returns the draft's statements, or nil when the app has none, and the notes of
// migrate.Notes: E-MIG-003 at the line of each destructive statement. Its error joins every
// problem, each at the draft's line or at the draft: those of Read and migrate.Check, and
// E-MIG-006 for a draft of an app that declares no sqldb.
func Check(dir string, m manifest.Manifest) (stmts []string, notes []*errs.Error, err error) {
	text, err := Read(dir)
	if err != nil || text == nil {
		return nil, nil, err
	}
	if !m.SQLDB() {
		return nil, nil, errs.At(File, "E-MIG-006", "the app has a draft, but its permission list declares no sqldb",
			"declare sqldb in aicoded.yaml when the app keeps data in a database, or delete "+File)
	}
	for _, n := range migrate.Notes(text) {
		var coded *errs.Error
		if errors.As(n, &coded) {
			notes = append(notes, coded)
		}
	}
	stmts, err = migrate.Check(text)
	if err != nil {
		return nil, notes, placed(err)
	}
	return stmts, notes, nil
}

// placed returns err with every coded error in it that has no position placed at the draft.
func placed(err error) error {
	var out []error
	for _, e := range unjoin(err) {
		var coded *errs.Error
		if errors.As(e, &coded) && coded.Pos == "" {
			e = errs.At(File, coded.Code, coded.Msg, coded.Fix)
		}
		out = append(out, e)
	}
	return errors.Join(out...)
}

// unjoin returns the errors that err joins, or err alone.
func unjoin(err error) []error {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return joined.Unwrap()
	}
	return []error{err}
}

// unreadable is E-MIG-001 for a draft that cannot be read.
func unreadable(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return errs.At(File, "E-MIG-001", "the draft cannot be read: "+err.Error(), "make "+File+" a plain file of SQL statements inside the app's folder")
}

// tooLarge is E-MIG-001 for a draft of size bytes, more than it may hold.
func tooLarge(size int64) error {
	return errs.At(File, "E-MIG-001", fmt.Sprintf("the draft holds %d bytes, more than the %d it may", size, maxSize),
		"split the change: publish part of it, then write the rest in a new draft")
}
