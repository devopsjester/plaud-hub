package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	calauth "github.com/devopsjester/plaud-hub/internal/calendar/auth"
	"github.com/devopsjester/plaud-hub/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Save your Plaud API token to the config file",
	Long: `Interactively set up your Plaud API token.

The token is saved to ~/.config/plaud-hub/config.yaml.

To get the token:
  1. Open https://web.plaud.ai and sign in
  2. Open DevTools → Network tab → find any request to api.plaud.ai
  3. Copy the Authorization header value (without the "bearer " prefix)`,
	RunE: runAuthSetup,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authSetupCmd)
	authCmd.AddCommand(authSetupGoogleCmd)
	authCmd.AddCommand(authSetupReclaimCmd)
	authCmd.AddCommand(authSetupGitHubCmd)
}

func runAuthSetup(_ *cobra.Command, _ []string) error {
	fmt.Println("Enter your Plaud API token:")
	fmt.Println("(Find it in DevTools → Network → api.plaud.ai request → Authorization header)")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input received")
	}

	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	// Remove "bearer " prefix if the user included it.
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimPrefix(token, "Bearer ")

	path, err := config.SaveToken(token)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	fmt.Printf("Token saved to %s\n", path)
	return nil
}

var authSetupGoogleCmd = &cobra.Command{
	Use:   "setup-google",
	Short: "Authorize plaud-hub to read your Google Calendar",
	Long: `Runs the OAuth 2.0 device-code flow for Google Calendar.

Reads client_id and client_secret from:
  ~/.config/plaud-hub/plaud-hub.yaml  (keys: calendar.google.client_id / client_secret)

On success, stores the access and refresh tokens in the same config file.`,
	RunE: runAuthSetupGoogle,
}

func runAuthSetupGoogle(_ *cobra.Command, _ []string) error {
	clientID := viper.GetString("calendar.google.client_id")
	clientSecret := viper.GetString("calendar.google.client_secret")

	if clientID == "" {
		return fmt.Errorf("calendar.google.client_id not set in config — add it to ~/.config/plaud-hub/plaud-hub.yaml")
	}
	if clientSecret == "" {
		return fmt.Errorf("calendar.google.client_secret not set in config — add it to ~/.config/plaud-hub/plaud-hub.yaml")
	}

	fmt.Println("Starting Google Calendar authorization...")

	accessToken, refreshToken, err := calauth.AuthorizeGoogle(context.Background(), clientID, clientSecret)
	if err != nil {
		return fmt.Errorf("Google authorization failed: %w", err)
	}

	if err := config.SaveCalendarToken("google", accessToken, refreshToken); err != nil {
		return fmt.Errorf("save tokens: %w", err)
	}

	fmt.Println("Google Calendar authorized successfully.")
	return nil
}

var authSetupReclaimCmd = &cobra.Command{
	Use:   "setup-reclaim",
	Short: "Save your Reclaim.ai API key to the config file",
	Long: `Stores your Reclaim.ai API key for use with --calendar reclaim.

Get your API key from: https://app.reclaim.ai/settings/developer`,
	RunE: runAuthSetupReclaim,
}

func runAuthSetupReclaim(_ *cobra.Command, _ []string) error {
	fmt.Println("Enter your Reclaim.ai API key:")
	fmt.Println("(Get it from https://app.reclaim.ai/settings/developer)")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input received")
	}

	key := strings.TrimSpace(scanner.Text())
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	if err := config.SaveReclaimKey(key); err != nil {
		return fmt.Errorf("save Reclaim API key: %w", err)
	}

	fmt.Println("Reclaim.ai API key saved.")
	return nil
}

var authSetupGitHubCmd = &cobra.Command{
	Use:   "setup-github",
	Short: "Save your GitHub personal access token to the config file",
	Long: `Stores a GitHub personal access token (PAT) for use with --split-llm github.

The token requires the "models: read" permission on GitHub.
Get a token from: https://github.com/settings/tokens

The token is saved to ~/.config/plaud-hub/plaud-hub.yaml with chmod 600.`,
	RunE: runAuthSetupGitHub,
}

func runAuthSetupGitHub(_ *cobra.Command, _ []string) error {
	fmt.Println("Enter your GitHub personal access token:")
	fmt.Println("(Requires 'models: read' permission — get one at https://github.com/settings/tokens)")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input received")
	}

	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	if err := config.SaveGitHubToken(token); err != nil {
		return fmt.Errorf("save GitHub token: %w", err)
	}

	configDir, _ := os.UserConfigDir()
	fmt.Printf("GitHub token saved to %s/plaud-hub/plaud-hub.yaml\n", configDir)
	return nil
}
