package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := cmdutil.NewGroup("auth", "Log in, log out, and show which login or API key is in use")
	cmd.AddCommand(newLoginCmd(f), newStatusCmd(f), newLogoutCmd(f))
	return cmd
}

// statusJSON is what auth status and auth login print with --json.
type statusJSON struct {
	Credential credentialJSON   `json:"credential"`
	BaseURL    string           `json:"base_url"`
	Account    *chronos.Account `json:"account"`
	// User is null for an API key.
	User *config.User `json:"user"`
}

type credentialJSON struct {
	Type     config.CredentialType `json:"type"`
	Source   config.Source         `json:"source"`
	Storage  *config.Storage       `json:"storage"`
	Prefix   string                `json:"prefix"`
	LastFour string                `json:"last_four"`
	// Scopes and ExpiresAt are null for an API key. ExpiresAt is also null for
	// a login the API gave no expiry.
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func statusOf(creds config.Credentials, account *chronos.Account) statusJSON {
	status := statusJSON{Credential: credentialOf(creds), BaseURL: creds.BaseURL, Account: account}
	if creds.Type == config.CredentialOAuthToken {
		user := creds.Grant.User
		status.User = &user
	}
	return status
}

// credentialOf shows an OAuth token by its last four characters only, since
// its prefix is the same for every token.
func credentialOf(creds config.Credentials) credentialJSON {
	c := credentialJSON{Type: creds.Type, Source: creds.Source}
	if creds.Storage != "" {
		c.Storage = &creds.Storage
	}
	if creds.Type != config.CredentialOAuthToken {
		c.Prefix, c.LastFour = keyParts(creds.Secret)
		return c
	}
	c.LastFour = lastFour(creds.Secret)
	c.Scopes = append([]string{}, creds.Grant.Scopes...)
	if !creds.Grant.ExpiresAt.IsZero() {
		c.ExpiresAt = &creds.Grant.ExpiresAt
	}
	return c
}

func statusView(creds config.Credentials, account *chronos.Account) output.View {
	return output.Resource(statusOf(creds, account), func(s statusJSON) []output.Field {
		fields := []output.Field{
			{Label: "Workspace", Value: s.Account.Name},
			{Label: "Workspace ID", Value: s.Account.ID},
		}
		if s.User != nil {
			fields = append(fields,
				output.Field{Label: "Logged in as", Value: s.User.Email},
				output.Field{Label: "OAuth token", Value: tokenHint(creds.Secret)},
				output.Field{Label: "Scopes", Value: strings.Join(s.Credential.Scopes, " ")},
				output.Field{Label: "Expires", Value: s.Credential.ExpiresAt},
			)
		} else {
			fields = append(fields, output.Field{Label: "API key", Value: keyHint(creds.Secret)})
		}
		return append(fields,
			output.Field{Label: "Source", Value: sourceText(creds)},
			output.Field{Label: "Base URL", Value: s.BaseURL},
		)
	})
}

func sourceText(creds config.Credentials) string {
	switch {
	case creds.Source == config.SourceEnv:
		return "CHRONOS_API_KEY"
	case creds.Storage == config.StorageKeychain:
		return "Saved in the keychain"
	}
	return "Saved in the config file"
}

// prefixLength matches the dashboard, which shows "chrns_" and three more
// characters.
const prefixLength = len("chrns_") + 3

// keyParts returns the prefix and last four characters of key, or nothing
// when those would show more than half of it.
func keyParts(key string) (prefix, lastFour string) {
	if len(key) < 2*(prefixLength+4) {
		return "", ""
	}
	return key[:prefixLength], key[len(key)-4:]
}

func keyHint(key string) string {
	prefix, lastFour := keyParts(key)
	if prefix == "" {
		return output.Masked
	}
	return prefix + output.Masked + lastFour
}

// lastFour returns "" for a token under eight characters, where four would
// show half of it or more.
func lastFour(token string) string {
	if len(token) < 8 {
		return ""
	}
	return token[len(token)-4:]
}

func tokenHint(token string) string {
	return output.Masked + lastFour(token)
}

func rejectedKey(err error, message string) error {
	apiErr, ok := errors.AsType[*chronos.APIError](err)
	if !ok || apiErr.StatusCode != http.StatusUnauthorized {
		return err
	}
	return exit.WithMessage(err, message)
}

func rejectedMessage(creds config.Credentials) string {
	if creds.Source == config.SourceEnv {
		return fmt.Sprintf("The API rejected the key in CHRONOS_API_KEY (%s). Set CHRONOS_API_KEY to a current key from the dashboard.", keyHint(creds.Secret))
	}
	return fmt.Sprintf("The API rejected your saved key (%s). Run chronos auth login to log in with the browser, or chronos auth login --paste-key with a current key from the dashboard.", keyHint(creds.Secret))
}
