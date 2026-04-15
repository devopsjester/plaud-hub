// Package config — calendar token storage helpers.
//
// Tokens are persisted under the same config file as the Plaud token, using
// Viper keys:
//
//	calendar.m365.access_token
//	calendar.m365.refresh_token
//	calendar.google.access_token
//	calendar.google.refresh_token
//
// The file is always written at chmod 600.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// SaveCalendarToken persists the access and refresh tokens for the given
// provider to the shared config file. provider must be "m365" or "google".
func SaveCalendarToken(provider, accessToken, refreshToken string) error {
	if err := validateProvider(provider); err != nil {
		return err
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("determine config directory: %w", err)
	}

	dir := filepath.Join(configDir, "plaud-hub")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	path := filepath.Join(dir, "plaud-hub.yaml")

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)
	_ = v.ReadInConfig() // ignore errors — file may not exist yet

	v.Set("calendar."+provider+".access_token", accessToken)
	v.Set("calendar."+provider+".refresh_token", refreshToken)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("set config permissions: %w", err)
	}

	return nil
}

// LoadCalendarToken reads the access and refresh tokens for the given provider
// from the config file. provider must be "m365" or "google".
// Returns empty strings without an error when no tokens have been stored yet.
func LoadCalendarToken(provider string) (accessToken, refreshToken string, err error) {
	if err := validateProvider(provider); err != nil {
		return "", "", err
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("determine config directory: %w", err)
	}

	path := filepath.Join(configDir, "plaud-hub", "plaud-hub.yaml")

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return "", "", nil
		}
		// Also treat a file-not-found OS error as "no tokens yet".
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("read config: %w", err)
	}

	return v.GetString("calendar." + provider + ".access_token"),
		v.GetString("calendar." + provider + ".refresh_token"),
		nil
}

// validateProvider returns an error for unrecognised provider strings.
// This is a defense-in-depth check; callers should use the named constants.
func validateProvider(provider string) error {
	switch provider {
	case "m365", "google":
		return nil
	default:
		return fmt.Errorf("unknown calendar provider %q: must be \"m365\" or \"google\"", provider)
	}
}
