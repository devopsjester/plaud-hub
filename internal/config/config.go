// Package config handles configuration loading via Viper.
//
// Precedence: CLI flag → environment variable → config file.
// Config file locations (in order): ./plaud-hub.yaml, ~/.config/plaud-hub/config.yaml
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Defaults for configuration values.
const (
	DefaultOutputDir   = "./output"
	DefaultConcurrency = 5
	DefaultType        = "all"

	// Subdirectory layout under DefaultOutputDir:
	//   output/
	//   ├── downloaded/              ← raw files from `download` command
	//   └── processed/
	//       ├── customers/{Name}/YYYY-MM/  ← matched customer content
	//       ├── internal/YYYY-MM/          ← LLM split "other" / internal notes
	//       └── unmatched/YYYY-MM/         ← recordings with no customer match
	SubdirDownloaded = "downloaded"
	SubdirProcessed  = "processed"
	SubdirCustomers  = "customers"
	SubdirInternal   = "internal"
	SubdirUnmatched  = "unmatched"
)

// Setup initializes Viper with config file paths and environment bindings.
// If configFile is non-empty, it is used exclusively; otherwise, standard
// search paths are used.
func Setup(configFile string) error {
	if configFile != "" {
		viper.SetConfigType("yaml")
		viper.SetConfigFile(configFile)
	} else {
		// Search in current directory first, then XDG config.
		viper.SetConfigName("plaud-hub")
		viper.AddConfigPath(".")

		configDir, err := os.UserConfigDir()
		if err == nil {
			viper.AddConfigPath(filepath.Join(configDir, "plaud-hub"))
		}
	}

	viper.SetEnvPrefix("PLAUD")
	viper.AutomaticEnv()

	// Bind specific keys to env vars.
	_ = viper.BindEnv("token", "PLAUD_TOKEN")

	// Set defaults.
	viper.SetDefault("output_dir", DefaultOutputDir)
	viper.SetDefault("concurrency", DefaultConcurrency)
	viper.SetDefault("type", DefaultType)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("read config: %w", err)
		}
		// Config file not found is fine — we'll use defaults + env + flags.
	}

	return nil
}

// Token returns the resolved API token from flag/env/config, or an error
// if no token is available.
func Token() (string, error) {
	token := viper.GetString("token")
	if token != "" {
		return token, nil
	}
	return "", fmt.Errorf(
		"Plaud token not found.\nOptions:\n" +
			"  1. Run:  plaud-hub auth setup\n" +
			"  2. Set:  export PLAUD_TOKEN='your-token'\n" +
			"  3. Add:  token: 'your-token' to config file\n",
	)
}

// SaveToken writes the token to the XDG config file at
// ~/.config/plaud-hub/config.yaml.
func SaveToken(token string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine config directory: %w", err)
	}

	dir := filepath.Join(configDir, "plaud-hub")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	path := filepath.Join(dir, "plaud-hub.yaml")

	// Read existing config if present, update token.
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(path)
	_ = v.ReadInConfig() // ignore errors — file may not exist
	v.Set("token", token)

	if err := v.WriteConfigAs(path); err != nil {
		return "", fmt.Errorf("write config: %w", err)
	}

	// Restrict permissions.
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("set config permissions: %w", err)
	}

	return path, nil
}
