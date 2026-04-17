// Package config — calendar token storage helpers.
//
// Google OAuth tokens are persisted under:
//
//	calendar.google.access_token
//	calendar.google.refresh_token
//
// Reclaim API key is persisted under:
//
//	calendar.reclaim.api_key
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
// provider to the shared config file. provider must be "google".
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
// from the config file. provider must be "google".
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
func validateProvider(provider string) error {
	if provider == "google" || provider == "reclaim" {
		return nil
	}
	return fmt.Errorf("unknown calendar provider %q: must be \"google\" or \"reclaim\"", provider)
}

// SaveReclaimKey persists the Reclaim.ai API key to the shared config file.
func SaveReclaimKey(apiKey string) error {
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
	_ = v.ReadInConfig()

	v.Set("calendar.reclaim.api_key", apiKey)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return os.Chmod(path, 0o600)
}

// LoadReclaimKey reads the Reclaim.ai API key. It first checks the global Viper
// instance (populated by config.Setup and settable in tests), then falls back to
// reading the config file directly.
// Returns an empty string without error when no key has been stored yet.
func LoadReclaimKey() (string, error) {
	// Global viper check first — works in production (after config.Setup) and tests.
	if key := viper.GetString("calendar.reclaim.api_key"); key != "" {
		return key, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config directory: %w", err)
	}

	path := filepath.Join(configDir, "plaud-hub", "plaud-hub.yaml")

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return "", nil
		}
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read config: %w", err)
	}
	return v.GetString("calendar.reclaim.api_key"), nil
}

// LoadGitHubToken reads the GitHub personal access token from the config file.
// The key is "github_token" at the root level (not nested under "calendar:").
// Returns an empty string without error when no token has been stored yet.
func LoadGitHubToken() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config directory: %w", err)
	}

	path := filepath.Join(configDir, "plaud-hub", "plaud-hub.yaml")

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return "", nil
		}
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read config: %w", err)
	}
	return v.GetString("github_token"), nil
}

// SaveGitHubToken persists the GitHub personal access token to the shared
// config file and enforces chmod 600 on the file.
func SaveGitHubToken(token string) error {
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

	v.Set("github_token", token)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return os.Chmod(path, 0o600)
}
