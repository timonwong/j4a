package config

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// OSSecretStore uses the platform's native credential manager.
type OSSecretStore struct{}

func (OSSecretStore) Get(service, account string) (string, error) {
	secret, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrSecretNotFound
	}
	return secret, err
}

func (OSSecretStore) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (OSSecretStore) Delete(service, account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrSecretNotFound
	}
	return err
}
