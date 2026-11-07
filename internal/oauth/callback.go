package oauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"chronos.sh/cli/internal/exit"
)

const shutdownTimeout = 5 * time.Second

var errLoginTimeout = errors.New("login timed out")

func (c *Client) authorize(ctx context.Context, verifier string, opts LoginOptions) (code, redirectURI string, err error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil && ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	if err != nil {
		return "", "", exit.Failuref("Could not listen on 127.0.0.1 for the browser login (%v). Log in with an API key instead: chronos auth login --paste-key.", err)
	}
	cb := &callback{
		host:    ln.Addr().String(),
		state:   rand.Text(),
		issuer:  c.issuer(),
		result:  make(chan callbackResult, 1),
		ignored: make(chan struct{}, 1),
	}
	srv := &http.Server{
		Handler:           cb,
		ReadHeaderTimeout: shutdownTimeout,
		ReadTimeout:       shutdownTimeout,
		WriteTimeout:      shutdownTimeout,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	srv.SetKeepAlivesEnabled(false)
	served := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = srv.Serve(ln)
		close(served)
	}()
	defer stop(srv, ln, served)

	ctx, cancel := context.WithTimeoutCause(ctx, opts.Timeout, errLoginTimeout)
	defer cancel()
	redirectURI = "http://" + cb.host + "/callback"
	opts.OpenBrowser(c.authorizeURL(authorizeRequest{redirectURI: redirectURI, state: cb.state, verifier: verifier, opts: opts}))
	for {
		select {
		case r := <-cb.result:
			return r.code, redirectURI, r.err
		case <-cb.ignored:
			opts.Ignored()
		case <-served:
			return "", "", exit.Failuref("The browser login listener stopped (%v), so nothing was saved. Run chronos auth login again.", serveErr)
		case <-ctx.Done():
			if errors.Is(context.Cause(ctx), errLoginTimeout) {
				return "", "", &exit.Error{
					Exit:      exit.Failure,
					Code:      exit.CodeError,
					Message:   "The login timed out before the browser returned to chronos, so nothing was saved. Run chronos auth login again, or save an API key from the dashboard with chronos auth login --paste-key.",
					Retryable: true,
				}
			}
			return "", "", ctx.Err()
		}
	}
}

// stop shuts the listener down and waits for Serve to return. Shutdown gets
// its own context, since the login's may already be cancelled.
func stop(srv *http.Server, ln net.Listener, served <-chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if srv.Shutdown(ctx) != nil {
		srv.Close()
	}
	<-served
	ln.Close()
}

type callbackResult struct {
	code string
	err  error
}

type callback struct {
	host     string
	state    string
	issuer   string
	answered atomic.Bool
	// result receives exactly one value, so it never blocks the handler.
	result  chan callbackResult
	ignored chan struct{}
}

func (cb *callback) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/callback" || r.Host != cb.host {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	if !cb.matchesState(q.Get("state")) || !cb.answered.CompareAndSwap(false, true) {
		select {
		case cb.ignored <- struct{}{}:
		default:
		}
		writePage(w, http.StatusBadRequest, ignoredPage)
		return
	}
	result := cb.read(q)
	cb.result <- result
	if result.err != nil {
		writePage(w, http.StatusBadRequest, failedPage)
		return
	}
	writePage(w, http.StatusOK, approvedPage)
}

func (cb *callback) matchesState(state string) bool {
	return subtle.ConstantTimeCompare([]byte(state), []byte(cb.state)) == 1
}

func (cb *callback) read(q url.Values) callbackResult {
	if iss := q.Get("iss"); iss != cb.issuer {
		return callbackResult{err: exit.Failuref("The browser returned a login that did not come from %s (iss %q), so nothing was saved. Run chronos auth login again.", cb.issuer, iss)}
	}
	if code := q.Get("error"); code != "" {
		if !knownError(code) {
			code = ""
		}
		return callbackResult{err: loginError(code, 0)}
	}
	code := q.Get("code")
	if code == "" {
		return callbackResult{err: exit.Failuref("The browser returned to chronos without a login code, so nothing was saved. Run chronos auth login again.")}
	}
	return callbackResult{code: code}
}

type page struct {
	Title string
	Text  string
}

var (
	approvedPage = page{"Login approved", "Return to your terminal to finish logging in to Chronos. You can close this tab."}
	failedPage   = page{"Login failed", "Nothing was saved. Return to your terminal to see why."}
	ignoredPage  = page{"Login not recognized", "chronos is not waiting for this login. If you started a new login, approve it in the tab chronos opened last."}
)

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>{{.Title}}</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:4rem auto;padding:0 1rem;line-height:1.5}</style>
</head>
<body><h1>{{.Title}}</h1><p>{{.Text}}</p></body>
</html>
`))

func writePage(w http.ResponseWriter, status int, p page) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = pageTemplate.Execute(w, p)
}
