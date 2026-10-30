// Package config stores saved API keys in the keychain or the config file and
// decides which API key and base URL a command uses.
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
	BaseURL    string     `json:"base_url,omitempty"`
	Credential Credential `json:"credential"`
}

type CredentialType string

const CredentialAPIKey CredentialType = "api_key"

type Storage string

const (
	StorageKeychain Storage = "keychain"
	StorageFile     Storage = "file"
)

// Credential points at a profile's secret. Secret is set only when Storage
// is StorageFile; otherwise the secret is in the keychain.
type Credential struct {
	Type    CredentialType `json:"type"`
	Storage Storage        `json:"storage"`
	Secret  string         `json:"secret,omitempty"`
}

func (p Profile) hasAPIKey() bool {
	if p.Credential.Type != CredentialAPIKey {
		return false
	}
	switch p.Credential.Storage {
	case StorageKeychain:
		return true
	case StorageFile:
		return p.Credential.Secret != ""
	}
	return false
}

func (p Profile) savedFor() string {
	if p.BaseURL == "" {
		return chronos.DefaultBaseURL
	}
	return p.BaseURL
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

func Path(dir string) string {
	return filepath.Join(dir, fileName)
}

func Empty() *File {
	return &File{Version: currentVersion, Profiles: map[string]Profile{}}
}

// Load reads the config file in dir. A missing file is an empty config. A
// config file or directory that other users can change is refused, since the
// file decides where the saved key is sent.
func Load(dir string) (*File, error) {
	path := Path(dir)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, exit.Failuref("%s is not a regular file, so chronos will not use it. Delete it and run chronos auth login again.", path)
	}
	if err := checkPrivateDir(dir); err != nil {
		return nil, err
	}
	if err := checkPrivate(path, info); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
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
	if f.holdsSecret() {
		if err := checkOthersCannotRead(path, info); err != nil {
			return nil, err
		}
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	return &f, nil
}

func (f *File) holdsSecret() bool {
	for _, p := range f.Profiles {
		if p.Credential.Storage == StorageFile {
			return true
		}
	}
	return false
}

// Save writes f to dir atomically, readable only by the current user.
func Save(dir string, f *File) error {
	if err := prepareDir(dir); err != nil {
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
	return os.Rename(tmp.Name(), Path(dir))
}

// prepareDir creates dir as 0700. An existing directory keeps its mode, since
// the user may share it read-only on purpose, but one that other users can
// write to is refused.
func prepareDir(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return checkPrivateDir(dir)
}

func checkPrivateDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	return checkPrivate(dir, info)
}

// Source says where an API key came from.
type Source string

const (
	SourceEnv   Source = "env"
	SourceSaved Source = "saved"
)

// Credentials are the key and base URL a command uses. APIKey is empty when
// no key is available, and for a key in the keychain until LoadSecret reads
// it.
type Credentials struct {
	APIKey  string
	BaseURL string
	Source  Source
	Storage Storage
	// SavedFor is set when a saved key exists but was not used because it
	// belongs to this other base URL.
	SavedFor string
}

// ResolveBaseURL is the --base-url flag, else CHRONOS_BASE_URL, else the
// default API.
func ResolveBaseURL(env Env, flag string) string {
	baseURL, _ := ResolveBaseURLSource(env, flag)
	return baseURL
}

// ResolveBaseURLSource is ResolveBaseURL plus the setting that chose it:
// "--base-url", "CHRONOS_BASE_URL", or "" for the default API.
func ResolveBaseURLSource(env Env, flag string) (baseURL, source string) {
	if flag != "" {
		return flag, "--base-url"
	}
	if v := env("CHRONOS_BASE_URL"); v != "" {
		return v, "CHRONOS_BASE_URL"
	}
	return chronos.DefaultBaseURL, ""
}

func IsDefaultBaseURL(baseURL string) bool {
	return normalizeURL(baseURL) == chronos.DefaultBaseURL
}

func EnvAPIKey(env Env) string {
	return strings.TrimSpace(env("CHRONOS_API_KEY"))
}

// Resolve picks the key and base URL. CHRONOS_API_KEY beats the saved key.
// A saved key is only sent to the base URL it was saved for, so pointing
// --base-url or CHRONOS_BASE_URL elsewhere never leaks it.
func (f *File) Resolve(env Env, baseURLFlag string) Credentials {
	baseURL, source := ResolveBaseURLSource(env, baseURLFlag)
	if key := EnvAPIKey(env); key != "" {
		return Credentials{APIKey: key, BaseURL: baseURL, Source: SourceEnv}
	}
	profile, saved := f.Profiles[DefaultProfile]
	if !saved || !profile.hasAPIKey() {
		return Credentials{BaseURL: baseURL}
	}
	savedFor := profile.savedFor()
	if source != "" && normalizeURL(baseURL) != normalizeURL(savedFor) {
		return Credentials{BaseURL: baseURL, SavedFor: savedFor}
	}
	return Credentials{
		APIKey:  profile.Credential.Secret,
		BaseURL: savedFor,
		Source:  SourceSaved,
		Storage: profile.Credential.Storage,
	}
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
