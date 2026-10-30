package clitest

import (
	"sync"

	"chronos.sh/cli/internal/config"
)

// Keychain is an in-memory keychain. With Err set, every call returns Err.
type Keychain struct {
	Err error

	mu    sync.Mutex
	items map[[2]string]string
}

func (k *Keychain) Get(service, account string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.Err != nil {
		return "", k.Err
	}
	secret, ok := k.items[[2]string{service, account}]
	if !ok {
		return "", config.ErrSecretNotFound
	}
	return secret, nil
}

func (k *Keychain) Set(service, account, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.Err != nil {
		return k.Err
	}
	if k.items == nil {
		k.items = map[[2]string]string{}
	}
	k.items[[2]string{service, account}] = secret
	return nil
}

func (k *Keychain) Delete(service, account string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.Err != nil {
		return k.Err
	}
	key := [2]string{service, account}
	if _, ok := k.items[key]; !ok {
		return config.ErrSecretNotFound
	}
	delete(k.items, key)
	return nil
}
