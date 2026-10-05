package filestore_test

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"aicoded.dev/framework/filestore"
	"aicoded.dev/framework/web/form"
)

var invoices *filestore.Store // the store Open returned in OnStart

// Open a store once, in OnStart, and keep it in the app's deps for every page. A store is an
// fs.FS.
func ExampleOpen() {
	onStart := func(ctx context.Context) error {
		var err error
		invoices, err = filestore.Open(ctx, "invoices") // filestore:invoices in the permission list
		if err != nil {
			return err
		}
		files := 0
		err = fs.WalkDir(invoices, ".", func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files++
			}
			return err
		})
		if err != nil {
			return err
		}
		fmt.Println(files, "invoices")
		return nil
	}
	_ = onStart
}

// WriteFile stores an uploaded file, here in a form's Process hook, under a name the app chooses.
// The file appears, whole, only when the write succeeds, and replaces a file of that name. Check
// the upload's name, size and content first, as the guide guides/forms says.
func ExampleStore_WriteFile() {
	process := func(upload *form.FileHeader) error {
		f, err := upload.Open()
		if err != nil {
			return err
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		if err := invoices.Mkdir("2026"); err != nil {
			return err
		}
		return invoices.WriteFile("2026/42.pdf", data)
	}
	_ = process
}
