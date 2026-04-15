package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/devopsjester/plaud-hub/internal/config"
	"github.com/spf13/cobra"
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
