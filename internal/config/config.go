// Package config reads and writes the CLI's config file and decides which API
// key and base URL a command uses.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/exit"
)

const (
	fileName       = "config.json"
	currentVersion = 1

	DefaultProfile = "default"
)

// Env reads an environment variable. os.Getenv in production.
type Env func(string) string

type File struct {
	Version  int                `json:"version"`
	Profiles map[string]Profile `json:"profiles"`
}

// Profile is one saved login. BaseURL is empty for the default API.
type Profile struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url,omitempty"`
}

// Dir returns the config directory: $CHRONOS_CONFIG_DIR, then
// $XDG_CONFIG_HOME/chronos, then ~/.config/chronos (the user config directory
// on Windows).
func Dir(env Env) (string, error) {
	if dir := env("CHRONOS_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if dir := env("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "chronos"), nil
	}
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "chronos"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "chronos"), nil
}

func Empty() *File {
	return &File{Version: currentVersion, Profiles: map[string]Profile{}}
}

// Load reads the config file in dir. A missing file is an empty config.
func Load(dir string) (*File, error) {
	path := filepath.Join(dir, fileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, exit.Failuref("%s is not valid JSON (%v). Fix it, or delete it and run chronos auth login again.", path, err)
	}
	switch {
	case f.Version == 0:
		return nil, exit.Failuref("%s has no version, so it was not written by chronos. Delete it and run chronos auth login again.", path)
	case f.Version > currentVersion:
		return nil, exit.Failuref("%s was written by a newer chronos (config version %d). Upgrade chronos.", path, f.Version)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	return &f, nil
}

// Save writes f to dir atomically, readable only by the current user.
func Save(dir string, f *File) error {
	if err := ensurePrivateDir(dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, fileName+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, fileName))
}

// ensurePrivateDir creates dir as 0700. A directory that already exists keeps
// its mode, since the user may have chosen a shared one on purpose.
func ensurePrivateDir(dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}

// Source says where an API key came from.
type Source string

const (
	SourceEnv    Source = "env"
	SourceConfig Source = "config"
)

// Credentials are the key and base URL a command uses. APIKey is empty when
// no key is available.
type Credentials struct {
	APIKey  string
	BaseURL string
	Source  Source
	// SavedFor is set when a saved key exists but was not used because it
	// belongs to this other base URL.
	SavedFor string
}

// ResolveBaseURL is the --base-url flag, else CHRONOS_BASE_URL, else the
// default API.
func ResolveBaseURL(env Env, flag string) string {
	if override := overrideBaseURL(env, flag); override != "" {
		return override
	}
	return chronos.DefaultBaseURL
}

func overrideBaseURL(env Env, flag string) string {
	if flag != "" {
		return flag
	}
	return env("CHRONOS_BASE_URL")
}

// Resolve picks the key and base URL. CHRONOS_API_KEY beats the saved key.
// A saved key is only sent to the base URL it was saved for, so pointing
// --base-url or CHRONOS_BASE_URL elsewhere never leaks it.
func (f *File) Resolve(env Env, baseURLFlag string) Credentials {
	override := overrideBaseURL(env, baseURLFlag)
	if key := strings.TrimSpace(env("CHRONOS_API_KEY")); key != "" {
		return Credentials{APIKey: key, BaseURL: ResolveBaseURL(env, baseURLFlag), Source: SourceEnv}
	}
	profile, saved := f.Profiles[DefaultProfile]
	if !saved || profile.APIKey == "" {
		return Credentials{BaseURL: ResolveBaseURL(env, baseURLFlag)}
	}
	savedFor := profile.BaseURL
	if savedFor == "" {
		savedFor = chronos.DefaultBaseURL
	}
	if override != "" && normalizeURL(override) != normalizeURL(savedFor) {
		return Credentials{BaseURL: override, SavedFor: savedFor}
	}
	return Credentials{APIKey: profile.APIKey, BaseURL: savedFor, Source: SourceConfig}
}

func normalizeURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

// CheckBaseURL rejects plain HTTP to anything but this machine, since the API
// key would cross the network unencrypted.
func CheckBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return exit.Usagef("The base URL %q is not a valid URL.", raw)
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil
	}
	return exit.Usagef("The base URL %q must use https. Plain http is allowed only for localhost.", raw)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
