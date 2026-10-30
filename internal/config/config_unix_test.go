//go:build unix

package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"chronos.sh/cli/internal/config"
)

func TestLoadRefusesAFileThatIsNotRegularWithoutOpeningIt(t *testing.T) {
	fifo := func(t *testing.T) string {
		dir := t.TempDir()
		if err := syscall.Mkfifo(config.Path(dir), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	linkToFIFO := func(t *testing.T) string {
		target := fifo(t)
		dir := t.TempDir()
		if err := os.Symlink(config.Path(target), config.Path(dir)); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	for name, makeDir := range map[string]func(*testing.T) string{"FIFO": fifo, "symlink to a FIFO": linkToFIFO} {
		t.Run(name, func(t *testing.T) {
			dir := makeDir(t)
			done := make(chan error, 1)
			go func() {
				_, err := config.Load(dir)
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "not a regular file") {
					t.Errorf("Load = %v, want a refusal", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Load opened the FIFO and blocked")
			}
		})
	}
}

func TestLoadFollowsASymlinkToARegularFile(t *testing.T) {
	target := t.TempDir()
	if err := config.Save(target, config.Empty()); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(config.Path(target), filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Errorf("Load = %v, want the linked file loaded", err)
	}
}
