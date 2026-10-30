package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// Keychain stores secrets in the operating system's credential store: the
// macOS keychain, the Secret Service on Linux, or Windows Credential Manager.
// Get and Delete return ErrSecretNotFound for a missing item. Every call
// returns ErrNoKeychain on a machine that has no keychain, and
// ErrKeychainTimeout when the keychain does not answer.
type Keychain interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

var (
	ErrSecretNotFound  = errors.New("secret not found in the keychain")
	ErrNoKeychain      = errors.New("no keychain is available")
	ErrKeychainTimeout = errors.New("the keychain did not answer")
)

// KeychainService is the keychain service a key for baseURL is saved under,
// such as "chronos:api.chronos.sh". Changing it orphans every saved key.
func KeychainService(baseURL string) string {
	host := baseURL
	if u, err := url.Parse(normalizeURL(baseURL)); err == nil && u.Host != "" {
		host = u.Host
	}
	return "chronos:" + strings.ToLower(host)
}

func orSystem(k Keychain) Keychain {
	if k == nil {
		return systemKeychain{}
	}
	return k
}

// keychainTimeout bounds each call, since a Secret Service that never
// answers would otherwise hang the command.
const keychainTimeout = 10 * time.Second

type systemKeychain struct{}

func (systemKeychain) Get(service, account string) (string, error) {
	var secret string
	err := withTimeout(keychainTimeout, func() error {
		var err error
		secret, err = keyring.Get(service, account)
		return keychainError(err)
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}

func (systemKeychain) Set(service, account, secret string) error {
	return withTimeout(keychainTimeout, func() error {
		return keychainError(keyring.Set(service, account, secret))
	})
}

func (systemKeychain) Delete(service, account string) error {
	return withTimeout(keychainTimeout, func() error {
		return keychainError(keyring.Delete(service, account))
	})
}

func keychainError(err error) error {
	switch {
	case errors.Is(err, keyring.ErrNotFound):
		return ErrSecretNotFound
	case noKeychain(err):
		return fmt.Errorf("%w: %w", ErrNoKeychain, err)
	}
	return err
}

func noKeychain(err error) bool {
	return errors.Is(err, keyring.ErrUnsupportedPlatform) || noSecretService(err)
}

// withTimeout returns ErrKeychainTimeout if call has not returned within d.
// call keeps running, so a write may still land after that.
func withTimeout(d time.Duration, call func() error) error {
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return fmt.Errorf("%w within %s", ErrKeychainTimeout, d)
	}
}
