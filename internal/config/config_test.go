package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"chronos.sh/cli/internal/config"
)

func TestSaveThenLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "chronos")
	want := &config.File{Version: 1, Profiles: map[string]config.Profile{
		config.DefaultProfile: {APIKey: "chrns_abc", BaseURL: "https://staging.example.com"},
	}}
	if err := config.Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profiles[config.DefaultProfile] != want.Profiles[config.DefaultProfile] {
		t.Errorf("loaded %+v, want %+v", got.Profiles, want.Profiles)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("config dir holds %d files, want only config.json", len(entries))
	}
	if runtime.GOOS == "windows" {
		return
	}
	assertMode(t, dir, 0o700)
	assertMode(t, filepath.Join(dir, "config.json"), 0o600)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	f, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Profiles) != 0 {
		t.Errorf("profiles = %v, want none", f.Profiles)
	}
}

func TestLoadRejectsFilesItCannotUse(t *testing.T) {
	tests := map[string]string{
		`{"version":2,"profiles":{}}`: "Upgrade chronos",
		`{"profiles":{}}`:             "has no version",
		`{not json`:                   "not valid JSON",
	}
	for content, want := range tests {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o600)
		_, err := config.Load(dir)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Load(%s) = %v, want %q", content, err, want)
		}
	}
}

func TestDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the home fallback differs on Windows")
	}
	home, _ := os.UserHomeDir()
	tests := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"CHRONOS_CONFIG_DIR": "/c", "XDG_CONFIG_HOME": "/x"}, "/c"},
		{map[string]string{"XDG_CONFIG_HOME": "/x"}, "/x/chronos"},
		{nil, filepath.Join(home, ".config", "chronos")},
	}
	for _, tt := range tests {
		got, err := config.Dir(func(k string) string { return tt.env[k] })
		if err != nil || got != tt.want {
			t.Errorf("Dir(%v) = %q, %v; want %q", tt.env, got, err, tt.want)
		}
	}
}

func TestResolve(t *testing.T) {
	stagingKey := &config.File{Version: 1, Profiles: map[string]config.Profile{
		config.DefaultProfile: {APIKey: "chrns_staging", BaseURL: "https://staging.example.com"},
	}}
	defaultKey := &config.File{Version: 1, Profiles: map[string]config.Profile{
		config.DefaultProfile: {APIKey: "chrns_prod"},
	}}
	tests := []struct {
		name string
		file *config.File
		env  map[string]string
		flag string
		want config.Credentials
	}{
		{
			name: "saved key keeps its base URL",
			file: stagingKey,
			want: config.Credentials{APIKey: "chrns_staging", BaseURL: "https://staging.example.com", Source: config.SourceConfig},
		},
		{
			name: "saved key with its own base URL given again",
			file: stagingKey,
			env:  map[string]string{"CHRONOS_BASE_URL": "https://staging.example.com/"},
			want: config.Credentials{APIKey: "chrns_staging", BaseURL: "https://staging.example.com", Source: config.SourceConfig},
		},
		{
			name: "saved key is not sent to another base URL",
			file: defaultKey,
			env:  map[string]string{"CHRONOS_BASE_URL": "http://localhost:3000"},
			want: config.Credentials{BaseURL: "http://localhost:3000", SavedFor: "https://api.chronos.sh"},
		},
		{
			name: "flag beats env, and the saved key stays home",
			file: stagingKey,
			env:  map[string]string{"CHRONOS_BASE_URL": "https://staging.example.com"},
			flag: "https://attacker.example",
			want: config.Credentials{BaseURL: "https://attacker.example", SavedFor: "https://staging.example.com"},
		},
		{
			name: "env key goes wherever the base URL points",
			file: stagingKey,
			env:  map[string]string{"CHRONOS_API_KEY": "chrns_env", "CHRONOS_BASE_URL": "http://localhost:3000"},
			want: config.Credentials{APIKey: "chrns_env", BaseURL: "http://localhost:3000", Source: config.SourceEnv},
		},
		{
			name: "env key with no base URL uses the default, not the saved one",
			file: stagingKey,
			env:  map[string]string{"CHRONOS_API_KEY": "chrns_env"},
			want: config.Credentials{APIKey: "chrns_env", BaseURL: "https://api.chronos.sh", Source: config.SourceEnv},
		},
		{
			name: "no key",
			file: config.Empty(),
			want: config.Credentials{BaseURL: "https://api.chronos.sh"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.file.Resolve(func(k string) string { return tt.env[k] }, tt.flag)
			if got != tt.want {
				t.Errorf("Resolve = %+v\nwant      %+v", got, tt.want)
			}
		})
	}
}

func TestCheckBaseURL(t *testing.T) {
	for _, ok := range []string{"https://api.chronos.sh", "http://localhost:3000", "http://127.0.0.1:8080", "http://[::1]:3000"} {
		if err := config.CheckBaseURL(ok); err != nil {
			t.Errorf("CheckBaseURL(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"http://api.chronos.sh", "http://10.0.0.5", "ftp://x", "not a url"} {
		if err := config.CheckBaseURL(bad); err == nil {
			t.Errorf("CheckBaseURL(%q) succeeded, want an error", bad)
		}
	}
}

func TestSaveKeepsAnExistingDirectorysMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions differ on Windows")
	}
	dir := filepath.Join(t.TempDir(), "shared")
	os.Mkdir(dir, 0o750)
	if err := config.Save(dir, config.Empty()); err != nil {
		t.Fatal(err)
	}
	assertMode(t, dir, 0o750)
	assertMode(t, filepath.Join(dir, "config.json"), 0o600)
}

func TestCheckBaseURLBypasses(t *testing.T) {
	for _, bad := range []string{"HTTP://evil.example", "http://localhost.attacker.example", "http://127.0.0.1.nip.io", "http://[::1%25lo0]:3000", "http://0.0.0.0", "//evil.com"} {
		if err := config.CheckBaseURL(bad); err == nil {
			t.Errorf("CheckBaseURL(%q) succeeded, want an error", bad)
		}
	}
}

func TestResolveTrimsTheEnvKey(t *testing.T) {
	saved := &config.File{Version: 1, Profiles: map[string]config.Profile{config.DefaultProfile: {APIKey: "chrns_saved"}}}
	got := saved.Resolve(func(k string) string { return map[string]string{"CHRONOS_API_KEY": "  \n"}[k] }, "")
	if got.APIKey != "chrns_saved" || got.Source != config.SourceConfig {
		t.Errorf("a blank env key hid the saved key: %+v", got)
	}
}
