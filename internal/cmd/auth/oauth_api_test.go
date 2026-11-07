package auth_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"chronos.sh/cli/internal/clitest"
)

const accountID = "0192f3a1-0000-7000-8000-000000000002"

// loopbackCallback is the only redirect URI form the API accepts.
var loopbackCallback = regexp.MustCompile(`^http://127\.0\.0\.1:[1-9][0-9]{0,4}/callback$`)

// oauthAPI behaves as the API's OAuth routes are documented. GET
// /oauth/authorize approves at once, as if the person had picked a workspace
// and clicked Approve, and redirects to the redirect URI with code, state and
// iss. POST /oauth/token redeems a code once against its PKCE challenge, POST
// /oauth/revoke always answers 200, and GET /v1/account answers 401 for a
// token that is unknown or revoked.
type oauthAPI struct {
	URL           string
	Issuer        string
	Deny          bool
	TokenError    string
	RevokeStatus  int
	RevokeError   string
	RevokeCutBody bool

	t        *testing.T
	mu       sync.Mutex
	codes    map[string]authorization
	tokens   map[string]bool
	requests []oauthRequest
}

type authorization struct {
	challenge   string
	redirectURI string
	scope       string
	used        bool
}

type oauthRequest struct {
	Method string
	Path   string
	Query  url.Values
	Form   url.Values
	Header http.Header
}

func newOAuthAPI(t *testing.T) *oauthAPI {
	t.Helper()
	api := &oauthAPI{t: t, codes: map[string]authorization{}, tokens: map[string]bool{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	api.URL = server.URL
	api.Issuer = server.URL
	return api
}

func (a *oauthAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, oauthRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Form: r.PostForm, Header: r.Header.Clone()})
	switch r.Method + " " + r.URL.Path {
	case "GET /oauth/authorize":
		a.authorize(w, r)
	case "POST /oauth/token":
		a.token(w, r)
	case "POST /oauth/revoke":
		switch {
		case a.RevokeError != "":
			oauthError(w, a.RevokeError)
		case a.RevokeStatus != 0:
			w.WriteHeader(a.RevokeStatus)
		case a.RevokeCutBody:
			delete(a.tokens, r.PostForm.Get("token"))
			cutBody(w)
		default:
			if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" && r.PostForm.Get("client_id") == "chronos-cli" {
				delete(a.tokens, r.PostForm.Get("token"))
			}
		}
	case "GET /v1/account":
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		res := clitest.Failure{Status: 401, Code: "unauthorized", Message: "Unauthorized"}.Response()
		if a.tokens[token] {
			res = account()
		}
		w.WriteHeader(res.Status)
		io.WriteString(w, res.Body)
	default:
		a.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusTeapot)
	}
}

func (a *oauthAPI) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if q.Get("client_id") != "chronos-cli" || err != nil || !loopbackCallback.MatchString(q.Get("redirect_uri")) {
		http.Redirect(w, r, "https://app.chronos.sh/oauth/consent?error=redirect_uri_invalid", http.StatusFound)
		return
	}
	reply := url.Values{"state": {q.Get("state")}, "iss": {a.Issuer}}
	switch {
	case q.Get("scope") == "":
		reply.Set("error", "invalid_scope")
	case q.Get("response_type") != "code", q.Get("code_challenge_method") != "S256", len(q.Get("code_challenge")) != 43, q.Get("state") == "", len(q.Get("state")) > 256, len(q.Get("device_name")) > 255:
		reply.Set("error", "invalid_request")
	case a.Deny:
		reply.Set("error", "access_denied")
	default:
		code := rand.Text()
		a.codes[code] = authorization{challenge: q.Get("code_challenge"), redirectURI: redirect.String(), scope: q.Get("scope")}
		reply.Set("code", code)
	}
	http.Redirect(w, r, redirect.String()+"?"+reply.Encode(), http.StatusFound)
}

func (a *oauthAPI) token(w http.ResponseWriter, r *http.Request) {
	form := r.PostForm
	code, found := a.codes[form.Get("code")]
	sum := sha256.Sum256([]byte(form.Get("code_verifier")))
	switch {
	case r.Header.Get("Content-Type") != "application/x-www-form-urlencoded", form.Get("client_id") != "chronos-cli":
		oauthError(w, "invalid_request")
	case form.Get("grant_type") != "authorization_code":
		oauthError(w, "unsupported_grant_type")
	case a.TokenError != "":
		oauthError(w, a.TokenError)
	case !found, code.used, code.redirectURI != form.Get("redirect_uri"), code.challenge != base64.RawURLEncoding.EncodeToString(sum[:]):
		oauthError(w, "invalid_grant")
	default:
		code.used = true
		a.codes[form.Get("code")] = code
		token := "chrns_oat_" + strings.ToLower(rand.Text()) + "x1Y2z3"
		a.tokens[token] = true
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   90 * 24 * 60 * 60,
			"scope":        code.scope,
			"user":         map[string]string{"id": "0192f3a1-0000-7000-8000-000000000009", "email": "ada@example.com", "name": "Ada Lovelace"},
			"account":      map[string]string{"id": accountID, "name": "Acme"},
		})
	}
}

func cutBody(w http.ResponseWriter) {
	w.Header().Set("Content-Length", "100")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "{")
	w.(http.Flusher).Flush()
	conn, _, err := http.NewResponseController(w).Hijack()
	if err == nil {
		conn.Close()
	}
}

func oauthError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": "Refused: " + code})
}

func (a *oauthAPI) Requests(method, path string) []oauthRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	var found []oauthRequest
	for _, r := range a.requests {
		if r.Method == method && r.Path == path {
			found = append(found, r)
		}
	}
	return found
}

// Tokens returns the tokens the API issued and has not revoked.
func (a *oauthAPI) Tokens() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var live []string
	for token := range a.tokens {
		live = append(live, token)
	}
	return live
}

type pageVisit struct {
	status int
	body   string
	header http.Header
}

// browser follows the authorization URL the way a browser does once the
// person approves: to the API, then to the redirect it answers with. Each
// visit to the loopback listener lands in visits.
func browser(t *testing.T, visits *[]pageVisit) func(string) error {
	return func(authorizeURL string) error {
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		res, err := client.Get(authorizeURL)
		if err != nil {
			t.Error(err)
			return nil
		}
		res.Body.Close()
		location := res.Header.Get("Location")
		if !strings.HasPrefix(location, "http://127.0.0.1:") {
			t.Errorf("the API redirected to %q, not to the loopback listener", location)
			return nil
		}
		*visits = append(*visits, visit(t, location))
		return nil
	}
}

func visit(t *testing.T, rawURL string) pageVisit {
	t.Helper()
	res, err := http.Get(rawURL)
	if err != nil {
		t.Error(err)
		return pageVisit{}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return pageVisit{status: res.StatusCode, body: string(body), header: res.Header}
}
