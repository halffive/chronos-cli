package auth_test

import (
	"strings"
	"testing"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

func TestStatusJSON(t *testing.T) {
	tests := []struct {
		name string
		save func(t *testing.T, dir string, keychain *clitest.Keychain, baseURL string)
	}{
		{name: "env"},
		{name: "keychain", save: func(t *testing.T, dir string, keychain *clitest.Keychain, baseURL string) {
			saveInKeychain(t, dir, keychain, baseURL, key)
		}},
		{name: "file", save: func(t *testing.T, dir string, _ *clitest.Keychain, baseURL string) {
			file := &config.File{Version: 1, Profiles: map[string]config.Profile{config.DefaultProfile: {
				BaseURL:    baseURL,
				Credential: config.Credential{Type: config.CredentialAPIKey, Storage: config.StorageFile, Secret: key},
			}}}
			if err := config.Save(dir, file); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := clitest.NewAPI(t, account())
			dir := t.TempDir()
			keychain := &clitest.Keychain{}
			env := map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir}
			if tt.save == nil {
				env["CHRONOS_API_KEY"] = key
			} else {
				tt.save(t, dir, keychain, api.URL)
			}
			res := clitest.Run(t, clitest.Options{Args: []string{"auth", "status", "--json"}, Env: env, Keychain: keychain})
			if res.Code != exit.OK {
				t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
			}
			golden(t, "status_"+tt.name, api, res.Stdout)
			assertNoKey(t, res)
		})
	}
}

func TestStatusHumanOutput(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	saveInKeychain(t, dir, keychain, api.URL, key)
	for name, tty := range map[string]bool{"terminal": true, "piped": false} {
		t.Run(name, func(t *testing.T) {
			res := clitest.Run(t, clitest.Options{
				Args:      []string{"auth", "status"},
				Env:       map[string]string{"CHRONOS_CONFIG_DIR": dir},
				StdoutTTY: tty,
				Keychain:  keychain,
			})
			if res.Code != exit.OK {
				t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
			}
			golden(t, "status_human_"+name, api, res.Stdout)
			assertNoKey(t, res)
		})
	}
}

func TestStatusEnvKeyBeatsTheSavedKey(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	saveInKeychain(t, dir, keychain, api.URL, "chrns_saved123456789abcdefghijklmnopq")
	res := clitest.Run(t, clitest.Options{
		Args:     []string{"auth", "status", "--json"},
		Env:      map[string]string{"CHRONOS_API_KEY": key, "CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir},
		Keychain: keychain,
	})
	if res.Code != exit.OK || !strings.Contains(res.Stdout, `"source":"env","storage":null`) {
		t.Errorf("exit = %d, stdout = %s", res.Code, res.Stdout)
	}
	if got := api.Requests()[0].Header.Get("Authorization"); got != "Bearer "+key {
		t.Errorf("sent %q, want the env key", got)
	}
}

func TestStatusTreatsABlankEnvKeyAsUnset(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	saveInKeychain(t, dir, keychain, api.URL, key)
	res := clitest.Run(t, clitest.Options{
		Args:     []string{"auth", "status", "--json"},
		Env:      map[string]string{"CHRONOS_API_KEY": " \n", "CHRONOS_CONFIG_DIR": dir},
		Keychain: keychain,
	})
	if res.Code != exit.OK || !strings.Contains(res.Stdout, `"source":"saved"`) {
		t.Errorf("exit = %d, stdout = %s, stderr = %s", res.Code, res.Stdout, res.Stderr)
	}
}

func TestStatusWithoutAKey(t *testing.T) {
	res := clitest.Run(t, clitest.Options{Args: []string{"auth", "status", "--json"}})
	got := decodeError(t, res.Stderr)
	if res.Code != exit.Auth || got.Code != exit.CodeAPIKeyMissing {
		t.Fatalf("exit = %d, error = %+v", res.Code, got)
	}
	if !strings.Contains(got.Message, "CHRONOS_API_KEY") || !strings.Contains(got.Message, "chronos auth login") {
		t.Errorf("message = %q, want it to name CHRONOS_API_KEY and chronos auth login", got.Message)
	}
}

func TestStatusWithASavedKeyMissingFromTheKeychain(t *testing.T) {
	dir := t.TempDir()
	saveInKeychain(t, dir, &clitest.Keychain{}, "https://api.chronos.sh", key)
	res := clitest.Run(t, clitest.Options{Args: []string{"auth", "status", "--json"}, Env: map[string]string{"CHRONOS_CONFIG_DIR": dir}})
	if got := decodeError(t, res.Stderr); res.Code != exit.Auth || got.Code != exit.CodeAPIKeyMissing {
		t.Errorf("exit = %d, error = %+v", res.Code, got)
	}
}

func TestStatusWithARejectedKey(t *testing.T) {
	api := clitest.NewAPI(t, unauthorized())
	res := clitest.Run(t, clitest.Options{Args: []string{"auth", "status", "--json"}, Env: api.Env()})
	got := decodeError(t, res.Stderr)
	if res.Code != exit.Auth || got.Code != "unauthorized" || got.Status == nil || *got.Status != 401 {
		t.Fatalf("exit = %d, error = %+v", res.Code, got)
	}
	golden(t, "status_rejected", api, res.Stderr)
	assertNoKey(t, res)
}
