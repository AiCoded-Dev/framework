// Package filestore reads and writes the app's file stores. The runner keeps the files; a store
// holds only this app's files, and no name can leave it.
//
// The permission list declares each store as a data source named filestore:<name>:
//
//	data:
//	  - source: filestore:invoices
//	    classes: [internal]
//
// [Open] returns the store as a [Store], which is an fs.FS, so fs.ReadFile and fs.WalkDir work
// with it. [Store.WriteFile] and [Store.Create] write a file, which appears, whole, only when the
// write succeeds; [Store.Mkdir], [Store.Remove] and [Store.Rename] change folders and names.
//
// Rules:
//
//   - Open each store once, in OnStart, and share it with the pages through the app's deps
//     package.
//   - Names are slash-separated paths inside the store, such as 2026/42.pdf, with no leading
//     slash and no . or .. parts (E-FILE-001).
//   - A file is at most 1 GiB (E-FILE-002).
//
// Read more in the guide docs/guides/file-stores.md and the task docs/tasks/upload-a-file.md, which
// aicoded explain and the MCP tool howto print as guides/file-stores and tasks/upload-a-file.
package filestore
