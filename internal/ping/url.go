// Package ping sends pings to a heartbeat monitor through the Ping API, a POST
// to the monitor's ping URL or to /start, /fail or /<exit-code> after it. The
// token in the URL is the only credential, so messages show the URL through
// Masked or MaskURL.
package ping

import (
	"errors"
	"net/url"
	"strings"

	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/output"
)

var (
	// ErrNotPingURL means the URL is not the origin followed by a token, such
	// as one copied with /start on the end.
	ErrNotPingURL = errors.New("not a ping URL")
	// ErrInsecure means plain http to a host other than this machine.
	ErrInsecure = errors.New("ping URL must use https")
	// ErrMaskedToken means the URL ends in a mask instead of the token, either
	// the API's **** for a key without monitors:write or the CLI's own.
	ErrMaskedToken = errors.New("ping URL token is masked")
)

const apiMask = "****"

// URL is a checked ping URL.
type URL struct {
	origin string
	token  string
}

// ParseURL accepts https://<host>/<token>, or plain http for localhost. Any
// other path, a query or a fragment is rejected rather than pinged.
func ParseURL(raw string) (*URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return nil, ErrNotPingURL
	}
	switch {
	case u.Scheme == "http" && !config.IsLoopback(u.Hostname()):
		return nil, ErrInsecure
	case u.Scheme != "https" && u.Scheme != "http":
		return nil, ErrNotPingURL
	case u.RawQuery != "" || u.Fragment != "" || u.ForceQuery:
		return nil, ErrNotPingURL
	}
	if u.Path == "/"+apiMask || u.Path == "/"+output.Masked {
		return nil, ErrMaskedToken
	}
	token, ok := strings.CutPrefix(u.EscapedPath(), "/")
	if !ok || token == "" || strings.Contains(token, "/") {
		return nil, ErrNotPingURL
	}
	return &URL{origin: u.Scheme + "://" + u.Host, token: token}, nil
}

// Masked is the URL with its token hidden.
func (u *URL) Masked() string {
	return u.origin + "/" + output.Masked
}

func (u *URL) endpoint(suffix string) string {
	if suffix == "" {
		return u.origin + "/" + u.token
	}
	return u.origin + "/" + u.token + "/" + suffix
}

// MaskURL hides every path segment of raw except masks and the Ping API's own
// suffixes, so a message about a URL ParseURL rejected never shows a token.
func MaskURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return output.Masked
	}
	segments := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	for i, s := range segments {
		if !shownSegment(s) {
			segments[i] = output.Masked
		}
	}
	return u.Scheme + "://" + u.Host + "/" + strings.Join(segments, "/")
}

func shownSegment(s string) bool {
	switch s {
	case "", "start", "fail", apiMask, output.Masked:
		return true
	}
	return strings.Trim(s, "0123456789") == ""
}
