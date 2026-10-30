package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

const key = "chrns_abc0123456789abcdefghijklmnopq"

func TestKeychainServiceNameIsFixed(t *testing.T) {
	tests := map[string]string{
		"https://api.chronos.sh":        "chronos:api.chronos.sh",
		"https://API.chronos.sh/":       "chronos:api.chronos.sh",
		"http://localhost:3000":         "chronos:localhost:3000",
		"https://staging.example.com/x": "chronos:staging.example.com",
	}
	for baseURL, want := range tests {
		if got := config.KeychainService(baseURL); got != want {
			t.Errorf("KeychainService(%q) = %q, want %q", baseURL, got, want)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSaveAPIKeyUsesTheKeychain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chronos")
	keychain := &clitest.Keychain{}
	saved, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh/", APIKey: key})
	if err != nil || saved.Warnings != nil {
		t.Fatalf("SaveAPIKey: %v, warnings: %q", err, saved.Warnings)
	}
	want := config.Credentials{APIKey: key, BaseURL: "https://api.chronos.sh", Source: config.SourceSaved, Storage: config.StorageKeychain}
	if saved.Credentials != want {
		t.Errorf("credentials = %+v, want %+v", saved.Credentials, want)
	}
	if got, _ := keychain.Get("chronos:api.chronos.sh", "default"); got != key {
		t.Errorf("keychain holds %q, want the key", got)
	}
	file := readFile(t, config.Path(dir))
	wantFile := `{
  "version": 1,
  "profiles": {
    "default": {
      "credential": {
        "type": "api_key",
        "storage": "keychain"
      }
    }
  }
}
`
	if file != wantFile {
		t.Errorf("config.json =\n%s\nwant\n%s", file, wantFile)
	}
}

func TestSaveAPIKeyFallsBackToTheFileWithoutAKeychain(t *testing.T) {
	dir := t.TempDir()
	noKeychain := &clitest.Keychain{Err: config.KeychainError(keyring.ErrUnsupportedPlatform)}
	saved, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: noKeychain, BaseURL: "https://staging.example.com", APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	reason := fmt.Sprintf("because no keychain is available (%v).", keyring.ErrUnsupportedPlatform)
	if saved.Credentials.Storage != config.StorageFile || len(saved.Warnings) != 1 || !strings.Contains(saved.Warnings[0], config.Path(dir)) || !strings.Contains(saved.Warnings[0], reason) {
		t.Errorf("result = %+v, want one warning naming the file and %q", saved, reason)
	}
	f, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Profile{BaseURL: "https://staging.example.com", Credential: fileKey(key)}
	if got := f.Profiles[config.DefaultProfile]; got != want {
		t.Errorf("profile = %+v, want %+v", got, want)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, config.Path(dir), 0o600)
	}
}

func TestSaveAPIKeyFailsWhenTheKeychainRefuses(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"dismissed prompt": {err: errors.New("prompt dismissed"), want: "refused"},
		"no answer":        {err: fmt.Errorf("%w within 10s", config.ErrKeychainTimeout), want: "If a keychain item with service chronos:api.chronos.sh appears later"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: &clitest.Keychain{Err: tt.err}, BaseURL: "https://api.chronos.sh", APIKey: key})
			e, ok := errors.AsType[*exit.Error](err)
			if !ok || e.Exit != exit.Failure || !strings.Contains(e.Message, tt.want) || !strings.Contains(e.Message, "CHRONOS_API_KEY") {
				t.Fatalf("err = %v, want exit 1 naming %q and CHRONOS_API_KEY", err, tt.want)
			}
			if _, err := os.Stat(config.Path(dir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("config.json was written: %v", err)
			}
		})
	}
}

func TestSaveAPIKeyWarnsAboutAnOldItemItCannotDelete(t *testing.T) {
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://staging.example.com", APIKey: key}); err != nil {
		t.Fatal(err)
	}
	keychain.Err = config.ErrNoKeychain
	saved, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Warnings) != 2 || !strings.Contains(saved.Warnings[1], "service chronos:staging.example.com and account default") {
		t.Errorf("warnings = %q, want one naming the staging item", saved.Warnings)
	}
}

func TestSaveAPIKeyDeletesAKeychainItemItReplaces(t *testing.T) {
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	save := func(baseURL string) {
		t.Helper()
		if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: baseURL, APIKey: key}); err != nil {
			t.Fatal(err)
		}
	}
	save("https://staging.example.com")
	save("https://api.chronos.sh")
	if _, err := keychain.Get("chronos:staging.example.com", "default"); !errors.Is(err, config.ErrSecretNotFound) {
		t.Errorf("the staging key is still in the keychain: %v", err)
	}
	save("https://api.chronos.sh")
	if got, _ := keychain.Get("chronos:api.chronos.sh", "default"); got != key {
		t.Errorf("saving to the same host again lost the key: %q", got)
	}
}

func TestSaveAPIKeyRestoresTheKeychainWhenTheFileCannotBeSaved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not block writes on Windows")
	}
	const newKey = "chrns_xyz0123456789abcdefghijklmnopq"
	tests := []struct {
		name     string
		previous bool
	}{
		{"replacing a saved key", true},
		{"first login", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			keychain := &clitest.Keychain{}
			if tt.previous {
				if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: key}); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(dir, 0o700) })
			if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: newKey}); err == nil {
				t.Fatal("save into a read-only directory succeeded")
			}
			got, err := keychain.Get("chronos:api.chronos.sh", "default")
			if tt.previous && got != key {
				t.Errorf("keychain holds %q, want the previous key back", got)
			}
			if !tt.previous && !errors.Is(err, config.ErrSecretNotFound) {
				t.Errorf("keychain holds %q, want no item", got)
			}
		})
	}
}

type unreadableKeychain struct{ *clitest.Keychain }

func (unreadableKeychain) Get(string, string) (string, error) {
	return "", config.ErrKeychainTimeout
}

func TestSaveAPIKeyKeepsAnItemItCouldNotReadWhenTheFileCannotBeSaved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory modes do not block writes on Windows")
	}
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: key}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	opts := config.SaveOptions{Dir: dir, Keychain: unreadableKeychain{keychain}, BaseURL: "https://api.chronos.sh", APIKey: "chrns_xyz0123456789abcdefghijklmnopq"}
	if _, err := config.SaveAPIKey(opts); err == nil {
		t.Fatal("save into a read-only directory succeeded")
	}
	if _, err := keychain.Get("chronos:api.chronos.sh", "default"); err != nil {
		t.Errorf("the saved profile's keychain item is gone: %v", err)
	}
}

func TestLoadSecret(t *testing.T) {
	keychain := &clitest.Keychain{}
	keychain.Set("chronos:api.chronos.sh", "default", key)
	saved := config.Credentials{BaseURL: "https://api.chronos.sh", Source: config.SourceSaved, Storage: config.StorageKeychain}

	got, err := config.LoadSecret(keychain, saved)
	if err != nil || got.APIKey != key {
		t.Errorf("LoadSecret = %+v, %v", got, err)
	}

	for name, k := range map[string]config.Keychain{
		"item deleted":         &clitest.Keychain{},
		"keychain unavailable": &clitest.Keychain{Err: errors.New("locked")},
	} {
		_, err := config.LoadSecret(k, saved)
		e, ok := errors.AsType[*exit.Error](err)
		if !ok || e.Exit != exit.Auth || e.Code != exit.CodeAPIKeyMissing {
			t.Errorf("%s: err = %v, want exit 4 %s", name, err, exit.CodeAPIKeyMissing)
		}
	}
}

func TestLoadSecretWhenTheKeychainIsUnreachable(t *testing.T) {
	saved := config.Credentials{BaseURL: "https://api.chronos.sh", Source: config.SourceSaved, Storage: config.StorageKeychain}
	for name, keychainErr := range map[string]error{
		"no keychain": config.KeychainError(keyring.ErrUnsupportedPlatform),
		"no answer":   fmt.Errorf("%w within 10s", config.ErrKeychainTimeout),
	} {
		_, err := config.LoadSecret(&clitest.Keychain{Err: keychainErr}, saved)
		e, ok := errors.AsType[*exit.Error](err)
		if !ok || e.Exit != exit.Auth || e.Code != exit.CodeAPIKeyMissing {
			t.Fatalf("%s: err = %v, want exit 4 %s", name, err, exit.CodeAPIKeyMissing)
		}
		if !strings.Contains(e.Message, "Could not reach the keychain") || strings.Contains(e.Message, "Unlock") {
			t.Errorf("%s: message = %q, want it to say the keychain is unreachable without suggesting an unlock", name, e.Message)
		}
	}
}

func TestRemoveProfile(t *testing.T) {
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: key}); err != nil {
		t.Fatal(err)
	}

	removed, err := config.RemoveProfile(dir, keychain)
	if err != nil || !removed.Removed || removed.Warnings != nil {
		t.Fatalf("RemoveProfile = %+v, %v", removed, err)
	}
	if _, err := keychain.Get("chronos:api.chronos.sh", "default"); !errors.Is(err, config.ErrSecretNotFound) {
		t.Errorf("the key is still in the keychain: %v", err)
	}
	if f, _ := config.Load(dir); len(f.Profiles) != 0 {
		t.Errorf("profiles = %+v, want none", f.Profiles)
	}

	removed, err = config.RemoveProfile(dir, keychain)
	if err != nil || removed.Removed {
		t.Errorf("second RemoveProfile = %+v, %v; want nothing removed", removed, err)
	}
}

func TestRemoveProfileWarnsWhenTheKeychainFails(t *testing.T) {
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	if _, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: key}); err != nil {
		t.Fatal(err)
	}
	keychain.Err = errors.New("locked")
	removed, err := config.RemoveProfile(dir, keychain)
	if err != nil || !removed.Removed {
		t.Fatalf("RemoveProfile = %+v, %v; want the profile removed", removed, err)
	}
	if len(removed.Warnings) != 1 || !strings.Contains(removed.Warnings[0], "service chronos:api.chronos.sh and account default") {
		t.Errorf("warnings = %q, want one naming the item", removed.Warnings)
	}
	if f, _ := config.Load(dir); len(f.Profiles) != 0 {
		t.Errorf("profiles = %+v, want none", f.Profiles)
	}
}

func TestRemoveProfileWithNothingSavedWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chronos")
	removed, err := config.RemoveProfile(dir, &clitest.Keychain{})
	if err != nil || removed.Removed {
		t.Errorf("RemoveProfile = %+v, %v", removed, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the config directory was created: %v", err)
	}
}

func TestLoadRefusesFilesOtherUsersCanChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions differ on Windows")
	}
	writable := func(t *testing.T, dirMode, fileMode os.FileMode) string {
		dir := t.TempDir()
		if err := config.Save(dir, config.Empty()); err != nil {
			t.Fatal(err)
		}
		os.Chmod(config.Path(dir), fileMode)
		os.Chmod(dir, dirMode)
		return dir
	}
	for name, dir := range map[string]string{
		"group-writable file":          writable(t, 0o700, 0o620),
		"world-writable directory":     writable(t, 0o777, 0o600),
		"group-writable directory":     writable(t, 0o770, 0o600),
		"world-writable both, as /tmp": writable(t, 0o1777, 0o666),
	} {
		_, err := config.Load(dir)
		if err == nil || !strings.Contains(err.Error(), "can be changed by other users") {
			t.Errorf("%s: Load = %v, want a refusal", name, err)
		}
	}
}

func TestSaveRefusesADirectoryOtherUsersCanChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions differ on Windows")
	}
	dir := filepath.Join(t.TempDir(), "shared")
	os.Mkdir(dir, 0o700)
	os.Chmod(dir, 0o770)
	keychain := &clitest.Keychain{}
	_, err := config.SaveAPIKey(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: "https://api.chronos.sh", APIKey: key})
	if err == nil {
		t.Fatal("SaveAPIKey succeeded, want a refusal")
	}
	if _, err := keychain.Get("chronos:api.chronos.sh", "default"); !errors.Is(err, config.ErrSecretNotFound) {
		t.Errorf("the key reached the keychain before the refusal: %v", err)
	}
}

func TestLoadRefusesASavedKeyOtherUsersCanRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions differ on Windows")
	}
	readable := func(t *testing.T, credential config.Credential) string {
		dir := t.TempDir()
		file := &config.File{Version: 1, Profiles: map[string]config.Profile{config.DefaultProfile: {Credential: credential}}}
		if err := config.Save(dir, file); err != nil {
			t.Fatal(err)
		}
		os.Chmod(config.Path(dir), 0o644)
		return dir
	}

	dir := readable(t, fileKey(key))
	_, err := config.Load(dir)
	if err == nil || !strings.Contains(err.Error(), "chmod 600 "+config.Path(dir)) {
		t.Errorf("key in the file: Load = %v, want a refusal with a chmod hint", err)
	}
	if _, err := config.Load(readable(t, keychainKey)); err != nil {
		t.Errorf("key in the keychain: Load = %v, want it loaded", err)
	}
}
