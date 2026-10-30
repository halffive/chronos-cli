package auth

import (
	"errors"
	"fmt"
	"net/http"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"

	"chronos.sh/cli/internal/app"
	"chronos.sh/cli/internal/cmdutil"
	"chronos.sh/cli/internal/config"
	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/output"
)

func NewCmd(f *app.Factory) *cobra.Command {
	cmd := cmdutil.NewGroup("auth", "Log in with an API key and check which key is in use")
	cmd.AddCommand(newLoginCmd(f), newStatusCmd(f), newLogoutCmd(f))
	return cmd
}

// statusJSON is what auth status and auth login print with --json.
type statusJSON struct {
	Credential credentialJSON   `json:"credential"`
	BaseURL    string           `json:"base_url"`
	Account    *chronos.Account `json:"account"`
	// User is always null for API keys.
	User any `json:"user"`
}

type credentialJSON struct {
	Type     config.CredentialType `json:"type"`
	Source   config.Source         `json:"source"`
	Storage  *config.Storage       `json:"storage"`
	Prefix   string                `json:"prefix"`
	LastFour string                `json:"last_four"`
}

func statusView(creds config.Credentials, account *chronos.Account) output.View {
	prefix, lastFour := keyParts(creds.APIKey)
	status := statusJSON{
		Credential: credentialJSON{
			Type:     config.CredentialAPIKey,
			Source:   creds.Source,
			Prefix:   prefix,
			LastFour: lastFour,
		},
		BaseURL: creds.BaseURL,
		Account: account,
	}
	if creds.Storage != "" {
		status.Credential.Storage = &creds.Storage
	}
	return output.Resource(status, func(s statusJSON) []output.Field {
		return []output.Field{
			{Label: "Workspace", Value: s.Account.Name},
			{Label: "Workspace ID", Value: s.Account.ID},
			{Label: "API key", Value: keyHint(creds.APIKey)},
			{Label: "Source", Value: sourceText(creds)},
			{Label: "Base URL", Value: s.BaseURL},
		}
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

func rejectedKey(err error, message string) error {
	apiErr, ok := errors.AsType[*chronos.APIError](err)
	if !ok || apiErr.StatusCode != http.StatusUnauthorized {
		return err
	}
	return exit.WithMessage(err, message)
}

func rejectedMessage(creds config.Credentials) string {
	if creds.Source == config.SourceEnv {
		return fmt.Sprintf("The API rejected the key in CHRONOS_API_KEY (%s). Set CHRONOS_API_KEY to a current key from the dashboard.", keyHint(creds.APIKey))
	}
	return fmt.Sprintf("The API rejected your saved key (%s). Run chronos auth login with a current key from the dashboard.", keyHint(creds.APIKey))
}
