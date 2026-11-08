package oauth

import (
	"encoding/json"
	"fmt"
	"net/http"

	chronos "chronos.sh/go"

	"chronos.sh/cli/internal/exit"
)

func errorCode(body []byte) string {
	var res struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &res) != nil || !knownError(res.Error) {
		return ""
	}
	return res.Error
}

func knownError(code string) bool {
	switch code {
	case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type",
		"invalid_scope", "access_denied", "unsupported_response_type", "unsupported_token_type",
		"server_error", "temporarily_unavailable":
		return true
	}
	return false
}

// loginError is the outcome of an error from the authorization redirect or
// the token endpoint. code is "" when the API sent none chronos knows, and
// status is 0 for the redirect.
func loginError(code string, status int) *exit.Error {
	switch {
	case code == "access_denied":
		return &exit.Error{Exit: exit.Failure, Code: code, Message: "The login was denied in the browser, so nothing was saved. Run chronos auth login to try again."}
	case code == "invalid_grant":
		return &exit.Error{Exit: exit.Auth, Code: code, Message: "The API did not accept the login code (invalid_grant), so nothing was saved. Run chronos auth login again."}
	case unavailable(code, status):
		return &exit.Error{Exit: exit.Retryable, Code: codeOrError(code), Message: fmt.Sprintf("The API could not finish the login (%s), so nothing was saved. Wait a moment, then run chronos auth login again.", reason(code, status))}
	}
	return &exit.Error{Exit: exit.Failure, Code: codeOrError(code), Message: fmt.Sprintf("The API refused the login (%s), so nothing was saved. Run chronos auth login again, or log in with an API key: chronos auth login --paste-key.", reason(code, status))}
}

// revokeError keeps the status and code as a Cause, so the error reports the
// status and logout can name the reason.
func revokeError(code string, status int) *exit.Error {
	e := &exit.Error{
		Exit:    exit.Failure,
		Code:    codeOrError(code),
		Message: fmt.Sprintf("The API refused to revoke the login (%s).", reason(code, status)),
		Cause:   &chronos.APIError{StatusCode: status, Code: chronos.ErrorCode(code)},
	}
	if unavailable(code, status) {
		e.Exit = exit.Retryable
	}
	return e
}

func unavailable(code string, status int) bool {
	switch {
	case code == "server_error", code == "temporarily_unavailable":
		return true
	case code != "":
		return false
	}
	return status == http.StatusTooManyRequests || status >= 500
}

func reason(code string, status int) string {
	switch {
	case code != "":
		return code
	case status != 0:
		return fmt.Sprintf("it answered %d", status)
	}
	return "no reason given"
}

func codeOrError(code string) string {
	if code == "" {
		return exit.CodeError
	}
	return code
}
