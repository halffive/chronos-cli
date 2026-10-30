package auth_test

import (
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

func TestLoginWithKeyFromStdin(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	res := clitest.Run(t, clitest.Options{
		Args:     []string{"auth", "login", "--with-key", "--json"},
		Env:      map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir},
		Stdin:    key + "\n",
		Keychain: keychain,
	})
	if res.Code != exit.OK {
		t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
	}
	golden(t, "login", api, res.Stdout)
	assertNoKey(t, res)

	requests := api.Requests()
	if len(requests) != 1 || requests[0].RequestURI != "/v1/account" || requests[0].Header.Get("Authorization") != "Bearer "+key {
		t.Errorf("requests = %+v, want one GET /v1/account with the key", requests)
	}
	if got, _ := keychain.Get(config.KeychainService(api.URL), "default"); got != key {
		t.Errorf("keychain holds %q, want the key", got)
	}
	if file := readConfig(t, dir); strings.Contains(file, key) {
		t.Errorf("config.json holds the key although the keychain worked:\n%s", file)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if info, err := os.Stat(config.Path(dir)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("config.json mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
}

func TestLoginOnATerminal(t *testing.T) {
	api := clitest.NewAPI(t, account())
	res := clitest.Run(t, clitest.Options{
		Args:      []string{"auth", "login"},
		Env:       map[string]string{"CHRONOS_BASE_URL": api.URL},
		StdinTTY:  true,
		StdoutTTY: true,
		Secret:    "  " + key + "  ",
	})
	if res.Code != exit.OK {
		t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
	}
	golden(t, "login_terminal", api, res.Stdout)
	golden(t, "login_terminal_stderr", api, res.Stderr)
	assertNoKey(t, res)
}

func TestLoginNeedsWithKeyWhenItCannotPrompt(t *testing.T) {
	tests := map[string]clitest.Options{
		"no terminal":          {Args: []string{"auth", "login"}},
		"--json on a terminal": {Args: []string{"auth", "login", "--json"}, StdinTTY: true},
	}
	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			dir := t.TempDir()
			opts.Env = map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir}
			opts.Stdin = key
			res := clitest.Run(t, opts)
			if res.Code != exit.Usage {
				t.Fatalf("exit = %d, want %d\nstderr: %s", res.Code, exit.Usage, res.Stderr)
			}
			if !strings.Contains(res.Stderr, "--with-key") || !strings.Contains(res.Stderr, "CHRONOS_API_KEY") {
				t.Errorf("stderr = %q, want it to name --with-key and CHRONOS_API_KEY", res.Stderr)
			}
			if _, err := os.Stat(config.Path(dir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("config.json was written: %v", err)
			}
		})
	}
}

func TestLoginRejectsInputThatIsNotAKey(t *testing.T) {
	for name, stdin := range map[string]string{
		"empty":     "",
		"blank":     " \n\t\n",
		"two lines": key + "\n" + key + "\n",
		"too long":  strings.Repeat("a", 2000),
	} {
		t.Run(name, func(t *testing.T) {
			api := clitest.NewAPI(t)
			res := clitest.Run(t, clitest.Options{
				Args:  []string{"auth", "login", "--with-key", "--json"},
				Env:   map[string]string{"CHRONOS_BASE_URL": api.URL},
				Stdin: stdin,
			})
			if got := decodeError(t, res.Stderr); res.Code != exit.Usage || got.Code != exit.CodeUsage {
				t.Errorf("exit = %d, error = %+v", res.Code, got)
			}
			assertNoKey(t, res)
		})
	}
}

func TestLoginWithARejectedKeyChangesNothing(t *testing.T) {
	api := clitest.NewAPI(t, unauthorized())
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	saveInKeychain(t, dir, keychain, api.URL, "chrns_old0123456789abcdefghijklmnopq")
	before := readConfig(t, dir)

	res := clitest.Run(t, clitest.Options{
		Args:     []string{"auth", "login", "--with-key", "--json"},
		Env:      map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir},
		Stdin:    key,
		Keychain: keychain,
	})
	got := decodeError(t, res.Stderr)
	if res.Code != exit.Auth || got.Code != "unauthorized" || got.RequestID == nil {
		t.Errorf("exit = %d, error = %+v", res.Code, got)
	}
	if res.Stdout != "" {
		t.Errorf("stdout = %q, want nothing", res.Stdout)
	}
	assertNoKey(t, res)
	if after := readConfig(t, dir); after != before {
		t.Errorf("config.json changed:\n%s", after)
	}
	if saved, _ := keychain.Get(config.KeychainService(api.URL), "default"); saved != "chrns_old0123456789abcdefghijklmnopq" {
		t.Errorf("keychain holds %q, want the old key", saved)
	}
}

func TestLoginWithoutAKeychainSavesToTheFile(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	env := map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir}
	noKeychain := &clitest.Keychain{Err: config.ErrNoKeychain}
	res := clitest.Run(t, clitest.Options{Args: []string{"auth", "login", "--with-key"}, Env: env, Stdin: key, Keychain: noKeychain})
	if res.Code != exit.OK {
		t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "CHRONOS_API_KEY") || !strings.Contains(res.Stderr, config.Path(dir)) {
		t.Errorf("stderr = %q, want a warning naming the file and CHRONOS_API_KEY", res.Stderr)
	}
	assertNoKey(t, res)

	status := clitest.Run(t, clitest.Options{Args: []string{"auth", "status", "--json"}, Env: env, Keychain: noKeychain})
	if status.Code != exit.OK || !strings.Contains(status.Stdout, `"source":"saved","storage":"file"`) {
		t.Errorf("status after a file login: exit %d, stdout %s, stderr %s", status.Code, status.Stdout, status.Stderr)
	}
}

func TestLoginFailsWhenTheKeychainRefuses(t *testing.T) {
	tests := map[string]struct {
		err  error
		want string
	}{
		"dismissed prompt": {err: errors.New("prompt dismissed"), want: "refused"},
		"no answer":        {err: config.ErrKeychainTimeout, want: "If a keychain item with service chronos:127.0.0.1"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			api := clitest.NewAPI(t, account())
			dir := t.TempDir()
			res := clitest.Run(t, clitest.Options{
				Args:     []string{"auth", "login", "--with-key"},
				Env:      map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir},
				Stdin:    key,
				Keychain: &clitest.Keychain{Err: tt.err},
			})
			if res.Code != exit.Failure || !strings.Contains(res.Stderr, tt.want) || !strings.Contains(res.Stderr, "CHRONOS_API_KEY") {
				t.Errorf("exit = %d, stderr = %q; want exit 1 naming %q and CHRONOS_API_KEY", res.Code, res.Stderr, tt.want)
			}
			if _, err := os.Stat(config.Path(dir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("config.json was written: %v", err)
			}
			assertNoKey(t, res)
		})
	}
}

func TestLoginWarnsAboutAnOldKeychainItemItCannotDelete(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	saveInKeychain(t, dir, keychain, "https://staging.example.com", "chrns_old0123456789abcdefghijklmnopq")
	keychain.Err = config.ErrNoKeychain
	res := clitest.Run(t, clitest.Options{
		Args:     []string{"auth", "login", "--with-key"},
		Env:      map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir},
		Stdin:    key,
		Keychain: keychain,
	})
	if res.Code != exit.OK || !strings.Contains(res.Stderr, "service chronos:staging.example.com and account default") {
		t.Errorf("exit = %d, stderr = %q; want a warning naming the staging item", res.Code, res.Stderr)
	}
}

func TestLoginWithKeyRefusesATerminal(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	res := clitest.Run(t, clitest.Options{
		Args:     []string{"auth", "login", "--with-key"},
		Env:      map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_CONFIG_DIR": dir},
		Stdin:    key,
		StdinTTY: true,
	})
	if res.Code != exit.Usage || !strings.Contains(res.Stderr, "chronos auth login --with-key < key.txt") {
		t.Errorf("exit = %d, stderr = %q", res.Code, res.Stderr)
	}
	if n := len(api.Requests()); n != 0 {
		t.Errorf("the API received %d requests, want 0", n)
	}
	if _, err := os.Stat(config.Path(dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.json was written: %v", err)
	}
}

func TestLoginNotesAnEnvKeyThatStillWins(t *testing.T) {
	api := clitest.NewAPI(t, account())
	res := clitest.Run(t, clitest.Options{
		Args:  []string{"auth", "login", "--with-key"},
		Env:   map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_API_KEY": "chrns_env0123456789abcdefghijklmnopq"},
		Stdin: key,
	})
	if res.Code != exit.OK || !strings.Contains(res.Stderr, "CHRONOS_API_KEY is set") {
		t.Errorf("exit = %d, stderr = %q", res.Code, res.Stderr)
	}
}

func TestLoginTreatsABlankEnvKeyAsUnset(t *testing.T) {
	api := clitest.NewAPI(t, account())
	res := clitest.Run(t, clitest.Options{
		Args:  []string{"auth", "login", "--with-key"},
		Env:   map[string]string{"CHRONOS_BASE_URL": api.URL, "CHRONOS_API_KEY": " \n"},
		Stdin: key,
	})
	if res.Code != exit.OK || strings.Contains(res.Stderr, "CHRONOS_API_KEY is set") {
		t.Errorf("exit = %d, stderr = %q", res.Code, res.Stderr)
	}
}

type hostRecorder struct{ hosts []string }

func (h *hostRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	h.hosts = append(h.hosts, r.URL.Host)
	body := account().Body
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func TestLoginNotesNothingForTheDefaultAPIGivenWithSpaces(t *testing.T) {
	recorder := &hostRecorder{}
	res := clitest.Run(t, clitest.Options{
		Args:          []string{"auth", "login", "--with-key"},
		Env:           map[string]string{"CHRONOS_BASE_URL": "https://api.chronos.sh/ \n"},
		Stdin:         key,
		ClientOptions: []chronos.Option{chronos.WithHTTPClient(&http.Client{Transport: recorder})},
	})
	if res.Code != exit.OK || res.Stderr != "" {
		t.Errorf("exit = %d, stderr = %q; want no note for the default API", res.Code, res.Stderr)
	}
	if len(recorder.hosts) != 1 || recorder.hosts[0] != "api.chronos.sh" {
		t.Errorf("the key was sent to %v, want only api.chronos.sh", recorder.hosts)
	}
}

func TestLoginIgnoresTheSavedBaseURL(t *testing.T) {
	dir := t.TempDir()
	saveInKeychain(t, dir, &clitest.Keychain{}, "https://staging.example.com", "chrns_old0123456789abcdefghijklmnopq")
	recorder := &hostRecorder{}
	res := clitest.Run(t, clitest.Options{
		Args:          []string{"auth", "login", "--with-key"},
		Env:           map[string]string{"CHRONOS_CONFIG_DIR": dir},
		Stdin:         key,
		ClientOptions: []chronos.Option{chronos.WithHTTPClient(&http.Client{Transport: recorder})},
	})
	if res.Code != exit.OK {
		t.Fatalf("exit = %d, stderr: %s", res.Code, res.Stderr)
	}
	if len(recorder.hosts) != 1 || recorder.hosts[0] != "api.chronos.sh" {
		t.Errorf("the key was sent to %v, want only api.chronos.sh", recorder.hosts)
	}
	if res.Stderr != "" {
		t.Errorf("stderr = %q, want no note for the default API", res.Stderr)
	}
}
