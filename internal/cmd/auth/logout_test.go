package auth_test

import (
	"errors"
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

func TestLogout(t *testing.T) {
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	saveInKeychain(t, dir, keychain, "https://api.chronos.sh", key)
	res := clitest.Run(t, clitest.Options{Args: []string{"auth", "logout", "--json"}, Env: map[string]string{"CHRONOS_CONFIG_DIR": dir}, Keychain: keychain})
	if res.Code != exit.OK || res.Stdout != `{"logged_out":true}`+"\n" || res.Stderr != "" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", res.Code, res.Stdout, res.Stderr)
	}
	if _, err := keychain.Get("chronos:api.chronos.sh", "default"); !errors.Is(err, config.ErrSecretNotFound) {
		t.Errorf("the key is still in the keychain: %v", err)
	}
	if f, _ := config.Load(dir); len(f.Profiles) != 0 {
		t.Errorf("profiles = %+v, want none", f.Profiles)
	}
}

func TestLogoutNotes(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		env    map[string]string
		stdout string
		stderr string
	}{
		{name: "nothing saved", args: []string{"auth", "logout"}, stderr: "There is no saved API key to remove.\n"},
		{name: "nothing saved, as JSON", args: []string{"auth", "logout", "--json"}, stdout: `{"logged_out":false}` + "\n"},
		{
			name:   "blank env key",
			args:   []string{"auth", "logout"},
			env:    map[string]string{"CHRONOS_API_KEY": " \n"},
			stderr: "There is no saved API key to remove.\n",
		},
		{
			name:   "env key set",
			args:   []string{"auth", "logout"},
			env:    map[string]string{"CHRONOS_API_KEY": key},
			stderr: "There is no saved API key to remove.\nCHRONOS_API_KEY is set, so commands still use that key.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := clitest.Run(t, clitest.Options{Args: tt.args, Env: tt.env})
			if res.Code != exit.OK || res.Stdout != tt.stdout || res.Stderr != tt.stderr {
				t.Errorf("exit = %d\nstdout = %q, want %q\nstderr = %q, want %q", res.Code, res.Stdout, tt.stdout, res.Stderr, tt.stderr)
			}
			assertNoKey(t, res)
		})
	}
}

func TestLogoutWhenTheKeychainFails(t *testing.T) {
	for name, args := range map[string][]string{"text": {"auth", "logout"}, "JSON": {"auth", "logout", "--json"}} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			keychain := &clitest.Keychain{}
			saveInKeychain(t, dir, keychain, "https://api.chronos.sh", key)
			keychain.Err = errors.New("locked")
			res := clitest.Run(t, clitest.Options{Args: args, Env: map[string]string{"CHRONOS_CONFIG_DIR": dir}, Keychain: keychain})
			if res.Code != exit.OK {
				t.Fatalf("exit = %d, stderr = %q", res.Code, res.Stderr)
			}
			if name == "JSON" && res.Stdout != `{"logged_out":true}`+"\n" {
				t.Errorf("stdout = %q", res.Stdout)
			}
			if name == "text" && !strings.Contains(res.Stderr, "service chronos:api.chronos.sh and account default") {
				t.Errorf("stderr = %q, want a warning naming the item", res.Stderr)
			}
			if f, _ := config.Load(dir); len(f.Profiles) != 0 {
				t.Errorf("profiles = %+v, want none", f.Profiles)
			}
		})
	}
}
