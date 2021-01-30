package auth

import (
	"errors"
	"fmt"
)

type hashType = string

const (
	Plain  hashType = "plain"
	MD5             = "md5"
	SHA256          = "sha256"
	Bcrypt          = "bcrypt"
)

var ValidateHashType = []string{
	Plain, MD5, SHA256, Bcrypt,
}

// Config is the configuration for the auth plugin.
type Config struct {
	// PasswordFile is the file to store username and password.
	PasswordFile string `yaml:"password_file"`
	// Hash is the password hash algorithm.
	// Possible values: plain | md5 | sha256 | bcrypt
	Hash string `yaml:"hash"`
}

// validate validates the configuration, and return an error if it is invalid.
func (c *Config) Validate() error {
	if c.PasswordFile == "" {
		return errors.New("password_file must be set")
	}
	for _, v := range ValidateHashType {
		if v == c.Hash {
			return nil
		}
	}
	return fmt.Errorf("invalid hash type: %s", c.Hash)
}

// DefaultConfig is the default configuration.
var DefaultConfig = Config{
	Hash:         MD5,
	PasswordFile: "./gmqtt_password.yml",
}

func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type cfg Config
	var v = &struct {
		Auth cfg `yaml:"auth"`
	}{
		Auth: cfg(DefaultConfig),
	}
	if err := unmarshal(v); err != nil {
		return err
	}
	empty := cfg(Config{})
	if v.Auth == empty {
		v.Auth = cfg(DefaultConfig)
	}
	*c = Config(v.Auth)
	return nil
}
