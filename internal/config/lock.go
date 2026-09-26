package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// The config file is shared: the CLI and the Trokky MCP server (@trokky/mcp) both read and
// write ~/.trokky/config.yaml, and a token refresh in one must not be lost to a concurrent
// save in the other. Both follow the same lock-file protocol before any read-modify-write,
// kept simple enough that the Node side implements it identically:
//
//   - take: create "<config>.lock" exclusively (O_EXCL) and write a random owner id into it
//   - wait: retry every 25ms, give up after lockGiveUpAfter
//   - release: remove the lock only if it still holds this owner's id
//   - stale: a lock older than lockStaleAfter was abandoned by a crashed process; take it
//     over by renaming it aside (only one waiter can), and put it back if it proves live
//
// A holder must finish well inside lockStaleAfter: the longest hold is a token refresh,
// bounded by a 30-second request timeout.
const (
	lockRetryEvery = 25 * time.Millisecond
	// Longer than the longest hold, so a waiter outlasts a slow refresh instead of failing
	lockGiveUpAfter = 45 * time.Second
	lockStaleAfter  = 60 * time.Second
)

func lockPath() string {
	return ConfigPath() + ".lock"
}

func newOwnerID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// acquireLock takes the config lock, returning a function that releases it.
func acquireLock() (func(), error) {
	path := lockPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	owner := newOwnerID()
	deadline := time.Now().Add(lockGiveUpAfter)

	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, werr := f.WriteString(owner)
			cerr := f.Close()
			if werr != nil || cerr != nil {
				_ = os.Remove(path)
				return nil, fmt.Errorf("cannot write %s", path)
			}
			return func() { releaseLock(path, owner) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("cannot lock %s: %w", path, err)
		}

		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			takeOverStaleLock(path)
			continue
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the Trokky config is locked by another process (%s); if nothing else is running, remove that file", path)
		}
		time.Sleep(lockRetryEvery)
	}
}

// releaseLock removes the lock only if it is still this owner's: after a stale takeover it
// belongs to someone else, and removing it would let a third process in alongside them.
func releaseLock(path, owner string) {
	data, err := os.ReadFile(path)
	if err == nil && string(data) == owner {
		_ = os.Remove(path)
	}
}

// takeOverStaleLock moves an abandoned lock aside. Renaming is atomic, so of several waiters
// that saw it stale only one moves it. If what was moved turns out to be fresh (another
// waiter replaced the stale lock between our stat and our rename), it is put back — with a
// link, which fails rather than overwrite a lock created meanwhile.
func takeOverStaleLock(path string) {
	aside := fmt.Sprintf("%s.%s.stale", path, newOwnerID())
	if os.Rename(path, aside) != nil {
		return
	}
	if info, err := os.Stat(aside); err == nil && time.Since(info.ModTime()) <= lockStaleAfter {
		_ = os.Link(aside, path)
	}
	_ = os.Remove(aside)
}

// Update runs a read-modify-write of the config under the lock. fn edits cfg in place; its
// changes are saved unless it returns an error.
func Update(fn func(cfg *Config) error) error {
	release, err := acquireLock()
	if err != nil {
		return err
	}
	defer release()

	cfg, err := Load()
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		return err
	}
	return Save(cfg)
}
