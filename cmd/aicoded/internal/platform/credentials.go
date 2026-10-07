package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"aicoded.dev/framework/internal/errs"
)

// Credentials are a builder's sign-in at one platform. Without tokens they are signed out, and
// Org is the organisation to sign in to again.
type Credentials struct {
	Org          string    `json:"org"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expires      time.Time `json:"expires,omitzero"`
}

// file is the credentials file: the credentials of each platform, by its address.
type file struct {
	Platforms map[string]Credentials `json:"platforms"`
}

// credentialsPath returns the credentials file, in the user's configuration folder.
func credentialsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the configuration folder: %w", err)
	}
	return filepath.Join(dir, "aicoded", "credentials.json"), nil
}

// Load returns the credentials kept for the platform at address, empty when there are none. A
// file that others can read is E-CLI-007.
func Load(address string) (Credentials, error) {
	f, _, err := read()
	return f.Platforms[address], err
}

// Save keeps cr as the credentials for the platform at address, in a file only the user can
// read, in a folder only the user can open, while it holds the lock on the file. A folder that
// others can open is E-CLI-007.
func Save(address string, cr Credentials) error {
	_, err := swap(address, cr)
	return err
}

// Delete deletes the credentials for the platform at address, and the file when it keeps no
// others, while it holds the lock on the file.
func Delete(address string) error {
	unlock, err := lock(context.Background())
	if err != nil {
		return err
	}
	defer unlock()
	return remove(address)
}

// swap keeps cr as the credentials for the platform at address, as Save does, and returns the
// ones it replaced.
func swap(address string, cr Credentials) (Credentials, error) {
	unlock, err := lock(context.Background())
	if err != nil {
		return Credentials{}, err
	}
	defer unlock()
	old, err := Load(address)
	if err != nil {
		return Credentials{}, err
	}
	return old, save(address, cr)
}

// save is Save for a caller that holds the lock.
func save(address string, cr Credentials) error {
	f, p, err := read()
	if err != nil {
		return err
	}
	if f.Platforms == nil {
		f.Platforms = map[string]Credentials{}
	}
	f.Platforms[address] = cr
	return write(p, f)
}

// remove is Delete for a caller that holds the lock.
func remove(address string) error {
	f, p, err := read()
	if err != nil {
		return err
	}
	if _, ok := f.Platforms[address]; !ok {
		return nil
	}
	delete(f.Platforms, address)
	if len(f.Platforms) == 0 {
		return os.Remove(p)
	}
	return write(p, f)
}

// lockWait is how long lock waits for another aicoded to release the lock.
var lockWait = 30 * time.Second

// lock takes the lock that aicoded holds while it changes the credentials file, or reads it to
// decide on a change: credentials.lock, next to it, of mode 0600. It waits at most lockWait for
// another aicoded to release it, and returns the function that releases it. A folder that others
// can open is E-CLI-007.
func lock(ctx context.Context) (unlock func(), err error) {
	p, err := credentialsPath()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(p)
	if err := folder(dir); err != nil {
		return nil, err
	}
	return lockFile(ctx, filepath.Join(dir, "credentials.lock"), lockWait)
}

// read reads the credentials file and returns it with its path. A missing file is empty.
func read() (file, string, error) {
	p, err := credentialsPath()
	if err != nil {
		return file{}, "", err
	}
	fh, err := os.Open(filepath.Clean(p))
	if errors.Is(err, fs.ErrNotExist) {
		return file{}, p, nil
	}
	if err != nil {
		return file{}, "", err
	}
	defer func() { _ = fh.Close() }()
	fi, err := fh.Stat()
	if err != nil {
		return file{}, "", err
	}
	if !fi.Mode().IsRegular() {
		return file{}, "", fmt.Errorf("%s is not a file: delete it, then run aicoded login", p)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return file{}, "", errs.New("E-CLI-007", fmt.Sprintf("%s keeps your sign-in to the platform, but others can read it (mode %04o)", p, perm),
			fmt.Sprintf("run chmod 600 %s, then aicoded logout and aicoded login, since others may have read the sign-in", p))
	}
	var f file
	if err := json.NewDecoder(io.LimitReader(fh, 1<<20)).Decode(&f); err != nil {
		return file{}, "", fmt.Errorf("%s is damaged: delete it, then run aicoded login", p)
	}
	return f, p, nil
}

// folder makes dir, the folder of the credentials file, when it is missing, and refuses it with
// E-CLI-007 when others can open it.
func folder(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return errs.New("E-CLI-007", fmt.Sprintf("%s keeps your sign-in to the platform, but others can open it (mode %04o)", dir, perm),
			"run chmod 700 "+dir)
	}
	return nil
}

// write writes f to p through a new file that replaces it, readable by the user only.
func write(p string, f file) error {
	dir := filepath.Dir(p)
	if err := folder(dir); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "credentials-*.json")
	if err != nil {
		return err
	}
	_, err = tmp.Write(append(b, '\n'))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), p)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("keep the sign-in: %w", err)
	}
	return nil
}
