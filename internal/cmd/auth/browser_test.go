package auth_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"chronos.sh/cli/internal/clitest"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
)

type browserLogin struct {
	api      *oauthAPI
	dir      string
	keychain *clitest.Keychain
	visits   []pageVisit
}

func newBrowserLogin(t *testing.T) *browserLogin {
	return &browserLogin{api: newOAuthAPI(t), dir: t.TempDir(), keychain: &clitest.Keychain{}}
}

func (b *browserLogin) env() map[string]string {
	return map[string]string{"CHRONOS_BASE_URL": b.api.URL, "CHRONOS_CONFIG_DIR": b.dir}
}

func (b *browserLogin) run(t *testing.T, opts clitest.Options) clitest.Result {
	t.Helper()
	opts.Env = b.env()
	opts.Keychain = b.keychain
	if opts.Args == nil {
		opts.Args = []string{"auth", "login"}
		opts.StdinTTY = true
	}
	if opts.Browser == nil {
		opts.Browser = browser(t, &b.visits)
	}
	return clitest.Run(t, opts)
}

func (b *browserLogin) login(t *testing.T) clitest.Result {
	t.Helper()
	res := b.run(t, clitest.Options{})
	clitest.WantExit(t, res, exit.OK)
	return res
}

func (b *browserLogin) assertNothingSaved(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(config.Path(b.dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.json was written: %v", err)
	}
	if _, err := b.keychain.Get(config.KeychainService(b.api.URL), "default"); !errors.Is(err, config.ErrSecretNotFound) {
		t.Errorf("the keychain holds a login: %v", err)
	}
}

func (b *browserLogin) assertLoggedOut(t *testing.T) {
	t.Helper()
	if f, err := config.Load(b.dir); err != nil || len(f.Profiles) != 0 {
		t.Errorf("profiles = %+v, %v; want none", f.Profiles, err)
	}
	if _, err := b.keychain.Get(config.KeychainService(b.api.URL), "default"); !errors.Is(err, config.ErrSecretNotFound) {
		t.Errorf("the keychain still holds the login: %v", err)
	}
}

func (b *browserLogin) assertNoToken(t *testing.T, res clitest.Result) {
	t.Helper()
	for _, token := range append(b.api.Tokens(), b.revokedTokens()...) {
		for name, out := range map[string]string{"stdout": res.Stdout, "stderr": res.Stderr} {
			if strings.Contains(out, token) || strings.Contains(out, token[len("chrns_oat_"):]) {
				t.Errorf("%s shows the token:\n%s", name, out)
			}
		}
	}
}

func (b *browserLogin) revokedTokens() []string {
	var tokens []string
	for _, r := range b.api.Requests("POST", "/oauth/revoke") {
		tokens = append(tokens, r.Form.Get("token"))
	}
	return tokens
}

func TestLoginWithTheBrowser(t *testing.T) {
	b := newBrowserLogin(t)
	res := b.login(t)
	clitest.Golden(t, "login_browser", res.Stdout)
	b.assertNoToken(t, res)
	for _, want := range []string{"Logging in to 127.0.0.1", "Opening the browser", b.api.URL + "/oauth/authorize?", "Waiting for you to approve"} {
		if !strings.Contains(res.Stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", res.Stderr, want)
		}
	}

	authorize := b.api.Requests("GET", "/oauth/authorize")
	if len(authorize) != 1 {
		t.Fatalf("authorize requests = %d, want 1", len(authorize))
	}
	q := authorize[0].Query
	if q.Get("scope") != "schedules:write jobs:write executions:read monitors:write account:read" || q.Get("device_name") == "" {
		t.Errorf("authorize query = %v", q)
	}
	token := b.api.Requests("POST", "/oauth/token")
	if len(token) != 1 || !strings.HasPrefix(token[0].Header.Get("User-Agent"), "chronos-cli/") {
		t.Errorf("token requests = %+v, want one with the CLI's user agent", token)
	}

	if len(b.visits) != 1 {
		t.Fatalf("the browser visited the listener %d times, want 1", len(b.visits))
	}
	page := b.visits[0]
	if page.status != 200 || !strings.Contains(page.body, "Login approved") || strings.Contains(page.body, "<script") {
		t.Errorf("page = %d %q", page.status, page.body)
	}
	if page.header.Get("Referrer-Policy") != "no-referrer" || !strings.HasPrefix(page.header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("page headers = %v", page.header)
	}

	tokens := b.api.Tokens()
	if len(tokens) != 1 {
		t.Fatalf("live tokens = %v, want 1", tokens)
	}
	if got, _ := b.keychain.Get(config.KeychainService(b.api.URL), "default"); got != tokens[0] {
		t.Errorf("keychain holds %q, want the token", got)
	}
	file := readConfig(t, b.dir)
	if strings.Contains(file, tokens[0]) {
		t.Errorf("config.json holds the token although the keychain worked:\n%s", file)
	}
	for _, want := range []string{`"type": "oauth_token"`, `"email": "ada@example.com"`, `"expires_at": "2027-01-26T12:00:00Z"`} {
		if !strings.Contains(file, want) {
			t.Errorf("config.json lacks %s:\n%s", want, file)
		}
	}
}

func TestStatusWithABrowserLogin(t *testing.T) {
	b := newBrowserLogin(t)
	b.login(t)
	asJSON := b.run(t, clitest.Options{Args: []string{"auth", "status", "--json"}})
	clitest.WantExit(t, asJSON, exit.OK)
	golden(t, "status_oauth", b.api.URL, asJSON.Stdout)
	b.assertNoToken(t, asJSON)

	terminal := b.run(t, clitest.Options{Args: []string{"auth", "status"}, StdoutTTY: true})
	clitest.WantExit(t, terminal, exit.OK)
	golden(t, "status_oauth_terminal", b.api.URL, terminal.Stdout)
	b.assertNoToken(t, terminal)
}

func TestBrowserLoginFailuresSaveNothing(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(b *browserLogin)
		browser func(t *testing.T, b *browserLogin) func(string) error
		exit    exit.Code
		message string
	}{
		{
			name:    "a login from another issuer",
			setup:   func(b *browserLogin) { b.api.Issuer = "https://attacker.example" },
			exit:    exit.Failure,
			message: `did not come from ` + "http://127.0.0.1",
		},
		{
			name:    "denied in the browser",
			setup:   func(b *browserLogin) { b.api.Deny = true },
			exit:    exit.Failure,
			message: "The login was denied in the browser, so nothing was saved.",
		},
		{
			name:    "a code the API refuses",
			setup:   func(b *browserLogin) { b.api.TokenError = "invalid_grant" },
			exit:    exit.Auth,
			message: "The API did not accept the login code",
		},
		{
			name: "no browser",
			browser: func(*testing.T, *browserLogin) func(string) error {
				return func(string) error { return errors.New("no display") }
			},
			exit:    exit.Failure,
			message: "The login timed out",
		},
		{
			name: "only a callback with another state",
			browser: func(t *testing.T, b *browserLogin) func(string) error {
				return func(authorizeURL string) error {
					u, _ := url.Parse(authorizeURL)
					q := url.Values{"state": {"not-this-login"}, "code": {"c"}, "iss": {b.api.URL}}
					b.visits = append(b.visits, visit(t, u.Query().Get("redirect_uri")+"?"+q.Encode()))
					return nil
				}
			},
			exit:    exit.Failure,
			message: "The login timed out",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBrowserLogin(t)
			if tt.setup != nil {
				tt.setup(b)
			}
			opts := clitest.Options{LoginTimeout: 200 * time.Millisecond}
			if tt.browser != nil {
				opts.Browser = tt.browser(t, b)
			}
			res := b.run(t, opts)
			clitest.WantExit(t, res, tt.exit)
			if !strings.Contains(res.Stderr, tt.message) {
				t.Errorf("stderr = %q, want it to contain %q", res.Stderr, tt.message)
			}
			if res.Stdout != "" {
				t.Errorf("stdout = %q, want nothing", res.Stdout)
			}
			b.assertNothingSaved(t)
			b.assertNoToken(t, res)
		})
	}
}

func TestBrowserLoginSaysWhenItIgnoresARequest(t *testing.T) {
	b := newBrowserLogin(t)
	approve := browser(t, &b.visits)
	res := b.run(t, clitest.Options{Browser: func(authorizeURL string) error {
		u, _ := url.Parse(authorizeURL)
		q := url.Values{"state": {"not-this-login"}, "code": {"c"}, "iss": {b.api.URL}}
		b.visits = append(b.visits, visit(t, u.Query().Get("redirect_uri")+"?"+q.Encode()))
		return approve(authorizeURL)
	}})
	clitest.WantExit(t, res, exit.OK)
	if b.visits[0].status != 400 || !strings.Contains(b.visits[0].body, "Login not recognized") {
		t.Errorf("first page = %d %q, want the page for a request that is not this login", b.visits[0].status, b.visits[0].body)
	}
	if res.Stdout != "Logged in as ada@example.com to Acme.\n" {
		t.Errorf("stdout = %q", res.Stdout)
	}
}

func TestBrowserLoginStopsOnInterrupt(t *testing.T) {
	b := newBrowserLogin(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan clitest.Result, 1)
	go func() {
		done <- b.run(t, clitest.Options{Context: ctx, LoginTimeout: time.Minute, Browser: func(string) error {
			cancel()
			return nil
		}})
	}()
	select {
	case res := <-done:
		clitest.WantExit(t, res, exit.Interrupted)
	case <-time.After(5 * time.Second):
		t.Fatal("auth login kept waiting for the browser after an interrupt")
	}
	b.assertNothingSaved(t)
}

func TestLogoutRevokesABrowserLogin(t *testing.T) {
	b := newBrowserLogin(t)
	b.login(t)
	issued := b.api.Tokens()[0]

	res := b.run(t, clitest.Options{Args: []string{"auth", "logout", "--json"}})
	if res.Code != exit.OK || res.Stdout != `{"logged_out":true}`+"\n" || res.Stderr != "" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", res.Code, res.Stdout, res.Stderr)
	}
	revoked := b.api.Requests("POST", "/oauth/revoke")
	if len(revoked) != 1 || revoked[0].Form.Get("token") != issued || revoked[0].Form.Get("client_id") != "chronos-cli" {
		t.Errorf("revoke requests = %+v, want one for the issued token", revoked)
	}
	b.assertLoggedOut(t)

	// Save the revoked token again to show the API now refuses it.
	if _, err := config.SaveCredential(config.SaveOptions{Dir: b.dir, Keychain: b.keychain, BaseURL: b.api.URL, Type: config.CredentialOAuthToken, Secret: issued}); err != nil {
		t.Fatal(err)
	}
	status := b.run(t, clitest.Options{Args: []string{"auth", "status", "--json"}})
	if got := clitest.DecodeError(t, status.Stderr); status.Code != exit.Auth || got.Code != exit.CodeLoginExpired || !strings.Contains(got.Message, "chronos auth login") {
		t.Errorf("status after logout: exit %d, error %+v", status.Code, got)
	}
}

func TestLogoutWhenTheAPIDoesNotRevoke(t *testing.T) {
	t.Run("a failure worth retrying keeps the login", func(t *testing.T) {
		b := newBrowserLogin(t)
		b.login(t)
		b.api.RevokeStatus = 503
		res := b.run(t, clitest.Options{Args: []string{"auth", "logout", "--json"}})
		got := clitest.DecodeError(t, res.Stderr)
		if res.Code != exit.Retryable || !got.Retryable || res.Stdout != "" || !strings.Contains(got.Message, "so it is still saved") || !strings.Contains(got.Message, "Run chronos auth logout again.") {
			t.Errorf("exit = %d, stdout = %q, error = %+v", res.Code, res.Stdout, got)
		}
		if _, err := b.keychain.Get(config.KeychainService(b.api.URL), "default"); err != nil {
			t.Errorf("the login was removed, so running logout again cannot revoke it: %v", err)
		}
		b.assertNoToken(t, res)
	})
	t.Run("a 200 whose body is cut short still counts", func(t *testing.T) {
		b := newBrowserLogin(t)
		b.login(t)
		b.api.RevokeCutBody = true
		res := b.run(t, clitest.Options{Args: []string{"auth", "logout", "--json"}})
		if res.Code != exit.OK || res.Stdout != `{"logged_out":true}`+"\n" {
			t.Errorf("exit = %d, stdout = %q, stderr = %q", res.Code, res.Stdout, res.Stderr)
		}
		b.assertLoggedOut(t)
	})
	t.Run("a refusal removes the login", func(t *testing.T) {
		b := newBrowserLogin(t)
		b.login(t)
		b.api.RevokeError = "invalid_client"
		res := b.run(t, clitest.Options{Args: []string{"auth", "logout", "--json"}})
		got := clitest.DecodeError(t, res.Stderr)
		if res.Code != exit.Failure || got.Code != "invalid_client" || !strings.Contains(got.Message, "could not revoke it on the server (invalid_client).") || !strings.Contains(got.Message, "CLI logins") {
			t.Errorf("exit = %d, error = %+v", res.Code, got)
		}
		if res.Stdout != `{"logged_out":true}`+"\n" {
			t.Errorf("stdout = %q", res.Stdout)
		}
		b.assertLoggedOut(t)
		b.assertNoToken(t, res)
	})
	t.Run("a locked keychain says so", func(t *testing.T) {
		b := newBrowserLogin(t)
		b.login(t)
		b.keychain.Err = errors.New("locked")
		res := b.run(t, clitest.Options{Args: []string{"auth", "logout", "--json"}})
		got := clitest.DecodeError(t, res.Stderr)
		if res.Code != exit.Auth || got.Code != exit.CodeAPIKeyMissing || !strings.Contains(got.Message, "keychain: locked)") || !strings.Contains(got.Message, "CLI logins") {
			t.Errorf("exit = %d, error = %+v", res.Code, got)
		}
		if f, err := config.Load(b.dir); err != nil || len(f.Profiles) != 0 {
			t.Errorf("profiles = %+v, %v; want none", f.Profiles, err)
		}
	})
}

func TestLoginRevokesTheLoginItReplaces(t *testing.T) {
	t.Run("a second browser login", func(t *testing.T) {
		b := newBrowserLogin(t)
		b.login(t)
		first := b.api.Tokens()[0]
		b.login(t)
		revoked := b.revokedTokens()
		if len(revoked) != 1 || revoked[0] != first {
			t.Errorf("revoked %q, want the first token", revoked)
		}
		if live := b.api.Tokens(); len(live) != 1 || live[0] == first {
			t.Errorf("live tokens = %q, want only the second", live)
		}
	})
	t.Run("an API key for another base URL", func(t *testing.T) {
		b := newBrowserLogin(t)
		b.login(t)
		first := b.api.Tokens()[0]
		other := clitest.NewAPI(t, account())
		res := clitest.Run(t, clitest.Options{
			Args:     []string{"auth", "login", "--with-key"},
			Env:      map[string]string{"CHRONOS_BASE_URL": other.URL, "CHRONOS_CONFIG_DIR": b.dir},
			Stdin:    key,
			Keychain: b.keychain,
		})
		clitest.WantExit(t, res, exit.OK)
		if revoked := b.revokedTokens(); len(revoked) != 1 || revoked[0] != first {
			t.Errorf("revoked %q, want the browser login's token at its own base URL", revoked)
		}
		if n := len(other.Requests()); n != 1 {
			t.Errorf("the new base URL got %d requests, want only the key check", n)
		}
	})
}

func TestBrowserLoginChecksTheConfigBeforeOpeningTheBrowser(t *testing.T) {
	b := newBrowserLogin(t)
	if err := os.WriteFile(config.Path(b.dir), []byte(`{"version":99,"profiles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res := b.run(t, clitest.Options{Browser: func(string) error {
		t.Error("the browser opened although the config cannot be used")
		return nil
	}})
	if res.Code != exit.Failure || !strings.Contains(res.Stderr, "newer chronos") {
		t.Errorf("exit = %d, stderr = %q", res.Code, res.Stderr)
	}
}

func TestLogoutInterruptedKeepsTheLogin(t *testing.T) {
	b := newBrowserLogin(t)
	b.login(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	res := b.run(t, clitest.Options{Args: []string{"auth", "logout"}, Context: ctx})
	clitest.WantExit(t, res, exit.Interrupted)
	if _, err := b.keychain.Get(config.KeychainService(b.api.URL), "default"); err != nil {
		t.Errorf("the login was removed although logout was interrupted: %v", err)
	}
}

func TestExpiredLoginFailsBeforeAnyRequest(t *testing.T) {
	api := clitest.NewAPI(t)
	dir := t.TempDir()
	keychain := &clitest.Keychain{}
	grant := config.Grant{User: config.User{Email: "ada@example.com"}, ExpiresAt: clitest.Now().Add(-time.Minute)}
	if _, err := config.SaveCredential(config.SaveOptions{Dir: dir, Keychain: keychain, BaseURL: api.URL, Type: config.CredentialOAuthToken, Secret: oauthToken, Grant: grant}); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, clitest.Options{Args: []string{"auth", "status", "--json"}, Env: map[string]string{"CHRONOS_CONFIG_DIR": dir}, Keychain: keychain})
	got := clitest.DecodeError(t, res.Stderr)
	if res.Code != exit.Auth || got.Code != exit.CodeLoginExpired || !strings.Contains(got.Message, "2026-10-28") {
		t.Errorf("exit = %d, error = %+v", res.Code, got)
	}
	if n := len(api.Requests()); n != 0 {
		t.Errorf("the API received %d requests, want 0", n)
	}
}

func TestTheTokenNeverPrints(t *testing.T) {
	api := clitest.NewAPI(t, account())
	dir := t.TempDir()
	file := &config.File{Version: 1, Profiles: map[string]config.Profile{config.DefaultProfile: {
		BaseURL: api.URL,
		Credential: config.Credential{Type: config.CredentialOAuthToken, Storage: config.StorageFile, Secret: oauthToken, Grant: config.Grant{
			User:    config.User{Email: "ada@example.com"},
			Account: config.Account{Name: "Acme"},
			Scopes:  []string{"jobs:write"},
		}},
	}}}
	if err := config.Save(dir, file); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"auth", "status", "--json"},
		{"auth", "status"},
		{"jobs", "cancel", "0192f3a2-0000-7000-8000-000000000003", "--dry-run", "--json"},
		{"jobs", "cancel", "0192f3a2-0000-7000-8000-000000000003", "--dry-run"},
	} {
		res := clitest.Run(t, clitest.Options{Args: args, Env: map[string]string{"CHRONOS_CONFIG_DIR": dir}, StdoutTTY: true})
		clitest.WantExit(t, res, exit.OK)
		for name, out := range map[string]string{"stdout": res.Stdout, "stderr": res.Stderr} {
			if strings.Contains(out, oauthToken) || strings.Contains(out, oauthToken[len("chrns_oat_"):len(oauthToken)-4]) {
				t.Errorf("%v: %s shows the token:\n%s", args, name, out)
			}
		}
	}
}
