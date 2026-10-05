package dev

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/devui"
	"aicoded.dev/framework/internal/errs"
)

// tokenFile is the file in the state folder that holds the dev UI's access token.
const tokenFile = "ui-token"

// readToken returns the dev UI's access token, which the state folder state keeps in a regular
// file of mode 0600 as 64 lowercase hex characters. With create, it first makes the file with a
// new random token when the file is missing. It refuses any other file there with E-DEV-018.
func readToken(state string, create bool) (string, error) {
	root, err := os.OpenRoot(state)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if create {
		if err := makeToken(root); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	path := filepath.Join(state, tokenFile)
	info, err := root.Lstat(tokenFile)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", badToken(path)
	}
	f, err := root.Open(tokenFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65))
	if err != nil {
		return "", err
	}
	if !devui.ValidToken(string(data)) {
		return "", badToken(path)
	}
	return string(data), nil
}

// makeToken writes a new random token to the token file in root, and fails with fs.ErrExist
// when something is at its path.
func makeToken(root *os.Root) error {
	f, err := root.OpenFile(tokenFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	_, err = f.WriteString(hex.EncodeToString(b))
	return errors.Join(err, f.Close())
}

func badToken(path string) error {
	return errs.New("E-DEV-018", path+" is not a regular file of mode 0600 that holds 64 hex characters",
		"delete "+path+"; aicoded dev makes a new token at its next start")
}

// loginLink returns the login link of the dev UI of the gateway at the address gateway, such as
// http://localhost:8080/, for token.
func loginLink(gateway, token string) string {
	return strings.TrimSuffix(gateway, "/") + devui.LoginPath + "?token=" + token
}
