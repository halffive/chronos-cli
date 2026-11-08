package oauth_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/oauth"
)

const issuer = "https://api.chronos.sh"

func TestChallengeMatchesRFC7636(t *testing.T) {
	if got := oauth.Challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("challenge = %q, want the RFC 7636 appendix B value", got)
	}
}

// browser plays the browser's part: it reads the authorization URL and
// requests the loopback listener directly. It may run on another goroutine,
// so it reports failures with Error, not Fatal.
type browser struct {
	t           *testing.T
	redirectURI string
	state       string
}

func newBrowser(t *testing.T, authorizeURL string) *browser {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Error(err)
	}
	q := u.Query()
	return &browser{t: t, redirectURI: q.Get("redirect_uri"), state: q.Get("state")}
}

func (b *browser) send(method, rawURL string, host string) int {
	b.t.Helper()
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		b.t.Error(err)
		return 0
	}
	if host != "" {
		req.Host = host
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	res, err := client.Do(req)
	if err != nil {
		b.t.Error(err)
		return 0
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode
}

// callback requests the redirect URI with params, filling in this login's
// state and the issuer unless params sets them.
func (b *browser) callback(params url.Values) int {
	b.t.Helper()
	q := url.Values{"state": {b.state}, "iss": {issuer}}
	for k, v := range params {
		q[k] = v
	}
	return b.send(http.MethodGet, b.redirectURI+"?"+q.Encode(), "")
}

func client(tokenURL string) *oauth.Client {
	return &oauth.Client{
		BaseURL:    tokenURL,
		HTTPClient: &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
	}
}

type loginRun struct {
	ctx     context.Context
	client  *oauth.Client
	timeout time.Duration
	browse  func(b *browser)
	ignored func()
}

func login(t *testing.T, r loginRun) (oauth.Token, error) {
	t.Helper()
	if r.ctx == nil {
		r.ctx = t.Context()
	}
	if r.client == nil {
		r.client = client(issuer)
	}
	if r.timeout == 0 {
		r.timeout = 5 * time.Second
	}
	if r.ignored == nil {
		r.ignored = func() { t.Error("Ignored was called") }
	}
	return r.client.Login(r.ctx, oauth.LoginOptions{
		Scope:       "account:read",
		Timeout:     r.timeout,
		OpenBrowser: func(authorizeURL string) { r.browse(newBrowser(t, authorizeURL)) },
		Ignored:     r.ignored,
	})
}

func wantOutcome(t *testing.T, err error, exitCode exit.Code, code string) {
	t.Helper()
	got := exit.Classify(err, exit.Sent{})
	if got.Exit != exitCode || got.Code != code {
		t.Errorf("outcome = exit %d code %q (%s), want exit %d code %q", got.Exit, got.Code, got.Message, exitCode, code)
	}
}

func TestLoginIgnoresRequestsThatAreNotItsCallback(t *testing.T) {
	ignored := make(chan struct{}, 2)
	browsed := make(chan struct{})
	_, err := login(t, loginRun{ignored: func() { ignored <- struct{}{} }, browse: func(b *browser) {
		go func() {
			defer close(browsed)
			sendStrays(t, b)
			select {
			case <-ignored:
			case <-time.After(5 * time.Second):
				t.Error("Ignored was not called for the bad state")
			}
			b.callback(url.Values{"error": {"access_denied"}})
		}()
	}})
	<-browsed
	wantOutcome(t, err, exit.Failure, "access_denied")
	if n := len(ignored); n != 0 {
		t.Errorf("Ignored was called %d more times, want once for the bad state", n)
	}
}

// sendStrays sends requests the listener must not take as the callback. Only
// the one with a bad state counts as ignored.
func sendStrays(t *testing.T, b *browser) {
	root := strings.TrimSuffix(b.redirectURI, "/callback")
	valid := b.redirectURI + "?" + url.Values{"state": {b.state}, "iss": {issuer}, "code": {"c"}}.Encode()
	for _, tt := range []struct {
		name   string
		method string
		url    string
		host   string
		want   int
	}{
		{"another path", http.MethodGet, root + "/favicon.ico", "", http.StatusNotFound},
		{"another method", http.MethodPost, valid, "", http.StatusMethodNotAllowed},
		{"another host", http.MethodGet, valid, "attacker.example", http.StatusNotFound},
	} {
		if got := b.send(tt.method, tt.url, tt.host); got != tt.want {
			t.Errorf("%s: status %d, want %d", tt.name, got, tt.want)
		}
	}
	if got := b.callback(url.Values{"state": {"wrong"}, "code": {"c"}}); got != http.StatusBadRequest {
		t.Errorf("bad state: status %d, want 400", got)
	}
}

func TestLoginTakesOnlyTheFirstCallback(t *testing.T) {
	_, err := login(t, loginRun{ignored: func() {}, browse: func(b *browser) {
		b.callback(url.Values{"error": {"access_denied"}})
		if got := b.callback(url.Values{"code": {"c"}}); got != http.StatusBadRequest {
			t.Errorf("second callback: status %d, want 400", got)
		}
	}})
	wantOutcome(t, err, exit.Failure, "access_denied")
}

func TestLoginChecksTheIssuerFirst(t *testing.T) {
	for name, params := range map[string]url.Values{
		"another issuer":            {"iss": {"https://attacker.example"}, "code": {"c"}},
		"no issuer":                 {"iss": {""}, "code": {"c"}},
		"an error from another one": {"iss": {"https://attacker.example"}, "error": {"access_denied"}},
		"a trailing slash":          {"iss": {issuer + "/"}, "code": {"c"}},
	} {
		t.Run(name, func(t *testing.T) {
			var page int
			_, err := login(t, loginRun{browse: func(b *browser) { page = b.callback(params) }})
			wantOutcome(t, err, exit.Failure, exit.CodeError)
			if err == nil || !strings.Contains(err.Error(), "nothing was saved") || !strings.Contains(err.Error(), issuer) {
				t.Errorf("error = %v, want it to name the issuer and say nothing was saved", err)
			}
			if page != http.StatusBadRequest {
				t.Errorf("page status = %d, want 400", page)
			}
		})
	}
}

func TestLoginComparesTheIssuerWithTheBaseURLsOrigin(t *testing.T) {
	for _, baseURL := range []string{"https://API.chronos.sh/", "https://api.chronos.sh:443", "https://api.chronos.sh/v1"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := login(t, loginRun{client: client(baseURL), browse: func(b *browser) {
				b.callback(url.Values{"error": {"access_denied"}})
			}})
			wantOutcome(t, err, exit.Failure, "access_denied")
		})
	}
}

func TestLoginTimesOut(t *testing.T) {
	_, err := login(t, loginRun{timeout: 20 * time.Millisecond, browse: func(*browser) {}})
	wantOutcome(t, err, exit.Failure, exit.CodeError)
	if err == nil || !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "--paste-key") {
		t.Errorf("error = %v, want a timeout that points to --paste-key", err)
	}
	assertStopped(t)
}

func TestLoginStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var redirectURI string
	done := make(chan error, 1)
	go func() {
		_, err := login(t, loginRun{ctx: ctx, browse: func(b *browser) {
			redirectURI = b.redirectURI
			cancel()
		}})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Login kept waiting after its context was cancelled")
	}
	u, _ := url.Parse(redirectURI)
	if conn, err := net.Dial("tcp", u.Host); err == nil {
		conn.Close()
		t.Errorf("the listener at %s still accepts connections", u.Host)
	}
	assertStopped(t)
}

func TestLoginExchangesTheCode(t *testing.T) {
	var form url.Values
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		form = r.PostForm
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, `{"access_token":"chrns_oat_abcdefghijklmnopqrstuvwxyz0Ab1Cd","token_type":"Bearer","expires_in":7776000,"scope":"schedules:write account:read","user":{"id":"u1","email":"ada@example.com","name":"Ada"},"account":{"id":"a1","name":"Acme"},"refresh":"ignored"}`)
	}))
	c := client(api.URL)
	var redirectURI string
	token, err := login(t, loginRun{client: c, browse: func(b *browser) {
		redirectURI = b.redirectURI
		q := url.Values{"state": {b.state}, "iss": {api.URL}, "code": {"the-code"}}
		b.send(http.MethodGet, b.redirectURI+"?"+q.Encode(), "")
	}})
	api.Close()
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "chrns_oat_abcdefghijklmnopqrstuvwxyz0Ab1Cd" || token.ExpiresIn != 90*24*time.Hour || len(token.Scopes) != 2 || token.User.Email != "ada@example.com" || token.Account.Name != "Acme" {
		t.Errorf("token = %+v", token)
	}
	verifier := form.Get("code_verifier")
	if form.Get("grant_type") != "authorization_code" || form.Get("code") != "the-code" || form.Get("redirect_uri") != redirectURI || form.Get("client_id") != oauth.ClientID || len(verifier) != 43 {
		t.Errorf("token request form = %v", form)
	}
	assertStopped(t)
}

func TestTokenErrors(t *testing.T) {
	const usable = `"token_type":"Bearer","user":{"email":"a@b.c"},"account":{"name":"A"}`
	tests := []struct {
		name    string
		status  int
		body    string
		exit    exit.Code
		code    string
		revoked string
	}{
		{name: "code already used", status: 400, body: `{"error":"invalid_grant","error_description":"Code is invalid"}`, exit: exit.Auth, code: "invalid_grant"},
		{name: "rate limited", status: 429, body: `{"error":"temporarily_unavailable","error_description":"Slow down"}`, exit: exit.Retryable, code: "temporarily_unavailable"},
		{name: "proxy error page", status: 502, body: `<html>Bad gateway</html>`, exit: exit.Retryable, code: exit.CodeError},
		{name: "unknown error code", status: 400, body: `{"error":"something_new"}`, exit: exit.Failure, code: exit.CodeError},
		{name: "client refused", status: 400, body: `{"error":"invalid_client"}`, exit: exit.Failure, code: "invalid_client"},
		{name: "redirect", status: 307, exit: exit.Failure, code: exit.CodeError},
		{name: "not a bearer token", status: 200, body: `{"access_token":"x","token_type":"mac","user":{"email":"a@b.c"},"account":{"name":"A"}}`, exit: exit.Failure, code: exit.CodeError, revoked: "x"},
		{name: "token with a newline", status: 200, body: `{"access_token":"a\nb",` + usable + `}`, exit: exit.Failure, code: exit.CodeError, revoked: "a\nb"},
		{name: "expiry that would overflow", status: 200, body: `{"access_token":"y","expires_in":9300000000000,` + usable + `}`, exit: exit.Failure, code: exit.CodeError, revoked: "y"},
		{name: "negative expiry", status: 200, body: `{"access_token":"z","expires_in":-1,` + usable + `}`, exit: exit.Failure, code: exit.CodeError, revoked: "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var revoked []string
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/revoke" {
					r.ParseForm()
					revoked = append(revoked, r.PostForm.Get("token"))
					return
				}
				if tt.status == 307 {
					w.Header().Set("Location", "https://attacker.example/steal")
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			defer api.Close()
			_, err := login(t, loginRun{client: client(api.URL), browse: func(b *browser) {
				q := url.Values{"state": {b.state}, "iss": {api.URL}, "code": {"c"}}
				b.send(http.MethodGet, b.redirectURI+"?"+q.Encode(), "")
			}})
			wantOutcome(t, err, tt.exit, tt.code)
			if want := tt.revoked; (want == "" && len(revoked) != 0) || (want != "" && (len(revoked) != 1 || revoked[0] != want)) {
				t.Errorf("revoked %q, want %q", revoked, want)
			}
		})
	}
}

func TestRevoke(t *testing.T) {
	var form url.Values
	status := http.StatusOK
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		form = r.PostForm
		w.WriteHeader(status)
	}))
	defer api.Close()
	c := client(api.URL)
	if err := c.Revoke(t.Context(), "chrns_oat_token"); err != nil {
		t.Fatal(err)
	}
	if form.Get("token") != "chrns_oat_token" || form.Get("client_id") != oauth.ClientID {
		t.Errorf("revoke form = %v", form)
	}
	status = http.StatusServiceUnavailable
	err := c.Revoke(t.Context(), "chrns_oat_token")
	wantOutcome(t, err, exit.Retryable, exit.CodeError)
	if err == nil || strings.Contains(err.Error(), "chrns_oat_token") {
		t.Errorf("err = %v, want an error that does not show the token", err)
	}
}

func TestRevokeBestEffortOutlivesACancelledContext(t *testing.T) {
	var revoked string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		revoked = r.PostForm.Get("token")
	}))
	defer api.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client(api.URL).RevokeBestEffort(ctx, "chrns_oat_token")
	if revoked != "chrns_oat_token" {
		t.Errorf("revoked %q, want the token although the context was cancelled", revoked)
	}
}

// assertStopped fails if a goroutine of this package, or a connection of any
// HTTP server, is still running. Connection goroutines finish just after the
// server lets go of them, so they get a moment.
func assertStopped(t *testing.T) {
	t.Helper()
	if stacks := goroutinesRunning("chronos.sh/cli/internal/oauth."); stacks != "" {
		t.Fatalf("goroutines still running after Login returned:\n%s", stacks)
	}
	deadline := time.Now().Add(time.Second)
	for {
		stacks := goroutinesRunning("net/http.(*conn).serve", "net/http.(*Server).Serve")
		if stacks == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("HTTP server goroutines still running after Login returned:\n%s", stacks)
		}
		time.Sleep(time.Millisecond)
	}
}

func goroutinesRunning(frames ...string) string {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	var found []string
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		for _, frame := range frames {
			if strings.Contains(g, frame) {
				found = append(found, g)
				break
			}
		}
	}
	return strings.Join(found, "\n\n")
}
