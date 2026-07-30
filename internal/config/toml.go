package config

import (
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type tomlFile struct {
	Default  tomlValues            `toml:"default"`
	Profiles map[string]tomlValues `toml:"profiles"`
}

type tomlValues struct {
	Host       *string `toml:"host"`
	Username   *string `toml:"username"`
	AuthType   *string `toml:"auth_type"`
	APIVersion *int    `toml:"api_version"`
	ReadOnly   *bool   `toml:"read_only"`
	UseKeyring *bool   `toml:"use_keyring"`
	Password   *string `toml:"password"`
	Token      *string `toml:"token"`
}

func parseTOML(input string) (fileConfig, error) {
	var decoded tomlFile
	decoder := toml.NewDecoder(strings.NewReader(input)).DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fileConfig{}, err
	}
	result := fileConfig{Default: decoded.Default.values(), Profiles: make(map[string]values, len(decoded.Profiles))}
	for name, profile := range decoded.Profiles {
		result.Profiles[name] = profile.values()
	}
	return result, nil
}

func (v tomlValues) values() values {
	return values{
		host: v.Host, username: v.Username, authType: v.AuthType,
		apiVersion: v.APIVersion, readOnly: v.ReadOnly, useKeyring: v.UseKeyring,
		password: v.Password, token: v.Token,
	}
}
