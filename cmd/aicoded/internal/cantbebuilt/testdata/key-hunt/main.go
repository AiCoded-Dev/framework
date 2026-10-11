// Command key-hunt looks for a database key the way a leaked-key exploit that runs inside an app
// would. It declares sqldb, so a runner that handed an app its database's address and password
// would have put them in its environment or its runner folder. It answers every request with
// what it found.
package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/sqldb"
)

func main() {
	app.Main(app.Options{Handler: http.HandlerFunc(hunt)})
}

// Found is what the app found: the names of its environment variables, without their values so
// that no answer can carry a key; the kind of every entry of its runner folder; and a row it
// wrote to its database and read back.
type Found struct {
	Env    []string
	Folder map[string]string
	Row    string
}

func hunt(w http.ResponseWriter, r *http.Request) {
	found := Found{Folder: map[string]string{}}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		found.Env = append(found.Env, name)
	}
	entries, err := os.ReadDir(os.Getenv("AICODED_RUNNER_DIR"))
	if err == nil {
		for _, e := range entries {
			found.Folder[e.Name()] = kind(e.Type())
		}
		found.Row, err = roundTrip(r.Context(), "written and read back")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(found)
}

// kind names the type of a folder entry.
func kind(t fs.FileMode) string {
	switch {
	case t&fs.ModeSocket != 0:
		return "socket"
	case t.IsDir():
		return "folder"
	case t.IsRegular():
		return "file"
	}
	return t.String()
}

// roundTrip writes row to the app's database and returns what it reads back.
func roundTrip(ctx context.Context, row string) (string, error) {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS found (id BIGINT AUTO_INCREMENT PRIMARY KEY, what VARCHAR(100) NOT NULL)"); err != nil {
		return "", err
	}
	res, err := db.ExecContext(ctx, "INSERT INTO found (what) VALUES (?)", row)
	if err != nil {
		return "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return "", err
	}
	var got string
	err = db.QueryRowContext(ctx, "SELECT what FROM found WHERE id = ?", id).Scan(&got)
	return got, err
}
