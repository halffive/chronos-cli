package config_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"chronos.sh/cli/internal/config"
)

func TestKeychainErrors(t *testing.T) {
	if err := config.KeychainError(keyring.ErrNotFound); err != config.ErrSecretNotFound {
		t.Errorf("not found = %v, want ErrSecretNotFound", err)
	}
	if err := config.KeychainError(keyring.ErrUnsupportedPlatform); !errors.Is(err, config.ErrNoKeychain) || !errors.Is(err, keyring.ErrUnsupportedPlatform) {
		t.Errorf("unsupported platform = %v, want ErrNoKeychain wrapping it", err)
	}
	refused := errors.New("failed to unlock correct collection '/org/freedesktop/secrets/aliases/default'")
	if err := config.KeychainError(refused); err != refused {
		t.Errorf("refused = %v, want it unchanged", err)
	}
	if err := config.KeychainError(nil); err != nil {
		t.Errorf("nil = %v", err)
	}
}

func TestKeychainTimeout(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	err := config.WithTimeout(10*time.Millisecond, func() error {
		<-release
		return nil
	})
	if !errors.Is(err, config.ErrKeychainTimeout) {
		t.Errorf("blocked call = %v, want ErrKeychainTimeout", err)
	}

	refused := errors.New("refused")
	if err := config.WithTimeout(time.Minute, func() error { return refused }); err != refused {
		t.Errorf("quick call = %v, want its own error", err)
	}
}
