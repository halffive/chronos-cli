package config

import (
	"errors"
	"fmt"
	"strings"

	"chronos.sh/cli/internal/exit"
)

// LoadSecret fills in APIKey for credentials saved in the keychain and
// returns other credentials unchanged.
func LoadSecret(k Keychain, c Credentials) (Credentials, error) {
	if c.Storage != StorageKeychain {
		return c, nil
	}
	key, err := orSystem(k).Get(KeychainService(c.BaseURL), DefaultProfile)
	switch {
	case errors.Is(err, ErrSecretNotFound):
		return Credentials{}, exit.MissingKeyf("Your saved key is no longer in the keychain. Run chronos auth login again, or set CHRONOS_API_KEY.")
	case errors.Is(err, ErrNoKeychain), errors.Is(err, ErrKeychainTimeout):
		return Credentials{}, exit.MissingKeyf("Could not reach the keychain to read your saved key (%v). Set CHRONOS_API_KEY, or run chronos auth login on a machine with a keychain.", err)
	case err != nil:
		return Credentials{}, exit.MissingKeyf("Could not read your saved key from the keychain (%v). Unlock the keychain and try again, or set CHRONOS_API_KEY.", err)
	}
	c.APIKey = key
	return c, nil
}

type SaveOptions struct {
	Dir      string
	Keychain Keychain
	BaseURL  string
	APIKey   string
}

type SaveResult struct {
	Credentials Credentials
	Warnings    []string
}

// SaveAPIKey saves the key as the default profile for the base URL: in the
// keychain, or in the config file on a machine that has no keychain. A
// keychain that refuses or does not answer fails the save. A keychain item
// the old profile no longer needs is deleted.
func SaveAPIKey(opts SaveOptions) (SaveResult, error) {
	file, err := Load(opts.Dir)
	if err != nil {
		return SaveResult{}, err
	}
	if err := prepareDir(opts.Dir); err != nil {
		return SaveResult{}, err
	}
	keychain := orSystem(opts.Keychain)
	baseURL := normalizeURL(opts.BaseURL)
	service := KeychainService(baseURL)

	old := file.Profiles[DefaultProfile]
	oldService := KeychainService(old.savedFor())
	undoSet := func() { _ = keychain.Delete(service, DefaultProfile) }
	if old.Credential.Storage == StorageKeychain && oldService == service {
		previous, err := keychain.Get(service, DefaultProfile)
		switch {
		case err == nil:
			undoSet = func() { _ = keychain.Set(service, DefaultProfile, previous) }
		case !errors.Is(err, ErrSecretNotFound):
			undoSet = func() {}
		}
	}

	result := SaveResult{Credentials: Credentials{APIKey: opts.APIKey, BaseURL: baseURL, Source: SourceSaved}}
	credential := Credential{Type: CredentialAPIKey, Storage: StorageKeychain}
	err = keychain.Set(service, DefaultProfile, opts.APIKey)
	switch {
	case errors.Is(err, ErrNoKeychain):
		credential.Storage, credential.Secret = StorageFile, opts.APIKey
		result.Warnings = append(result.Warnings, fmt.Sprintf("Saved the key in %s, readable only by you, because %s. To keep the key out of files, run chronos auth logout and set CHRONOS_API_KEY instead.", Path(opts.Dir), noKeychainReason(err)))
	case errors.Is(err, ErrKeychainTimeout):
		return SaveResult{}, exit.Failuref("The keychain did not answer within %s, so the key was not saved. If a keychain item with service %s appears later, run chronos auth login again to replace it or delete it in your keychain app. To skip the keychain, set CHRONOS_API_KEY instead of logging in.", keychainTimeout, service)
	case err != nil:
		return SaveResult{}, exit.Failuref("The keychain refused the key (%v), so it was not saved. Unlock the keychain and run chronos auth login again, or set CHRONOS_API_KEY instead of logging in.", err)
	}
	result.Credentials.Storage = credential.Storage

	profile := Profile{Credential: credential}
	if !IsDefaultBaseURL(baseURL) {
		profile.BaseURL = baseURL
	}
	file.Profiles[DefaultProfile] = profile
	if err := Save(opts.Dir, file); err != nil {
		if credential.Storage == StorageKeychain {
			undoSet()
		}
		return SaveResult{}, err
	}

	overwroteOldItem := credential.Storage == StorageKeychain && oldService == service
	if old.Credential.Storage == StorageKeychain && !overwroteOldItem {
		if warning := deleteItem(keychain, oldService); warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}
	return result, nil
}

func deleteItem(k Keychain, service string) (warning string) {
	err := k.Delete(service, DefaultProfile)
	if err == nil || errors.Is(err, ErrSecretNotFound) {
		return ""
	}
	return fmt.Sprintf("Could not delete the keychain item with service %s and account %s (%v). Delete it in your keychain app.", service, DefaultProfile, err)
}

func noKeychainReason(err error) string {
	cause, wrapped := strings.CutPrefix(err.Error(), ErrNoKeychain.Error()+": ")
	if !wrapped {
		return err.Error()
	}
	return fmt.Sprintf("%s (%s)", ErrNoKeychain, cause)
}

type RemoveResult struct {
	Removed  bool
	Warnings []string
}

// RemoveProfile deletes the default profile and its keychain item. A
// keychain item that cannot be deleted leaves a warning, not an error, so
// the profile is still removed.
func RemoveProfile(dir string, k Keychain) (RemoveResult, error) {
	file, err := Load(dir)
	if err != nil {
		return RemoveResult{}, err
	}
	profile, saved := file.Profiles[DefaultProfile]
	if !saved {
		return RemoveResult{}, nil
	}
	result := RemoveResult{Removed: true}
	if profile.Credential.Storage == StorageKeychain {
		if warning := deleteItem(orSystem(k), KeychainService(profile.savedFor())); warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}
	delete(file.Profiles, DefaultProfile)
	if err := Save(dir, file); err != nil {
		return RemoveResult{}, err
	}
	return result, nil
}
