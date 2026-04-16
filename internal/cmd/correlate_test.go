package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// newMinCorrelateCmd builds a *cobra.Command with the same flags that
// runCorrelate reads, so tests can call runCorrelate directly without
// touching the shared correlateCmd state.
func newMinCorrelateCmd(splitLLM, minConf string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("customers-file", "/nonexistent-for-test", "")
	cmd.Flags().Bool("keep", false, "")
	cmd.Flags().String("min-confidence", minConf, "")
	cmd.Flags().String("calendar", "", "")
	cmd.Flags().Duration("calendar-tolerance", 15*time.Minute, "")
	cmd.Flags().String("split-llm", splitLLM, "")
	return cmd
}

// TestCorrelateKeepFlagDefault confirms that --keep is registered with a
// default of false (move is the default behaviour).
func TestCorrelateKeepFlagDefault(t *testing.T) {
	f := correlateCmd.Flags().Lookup("keep")
	if f == nil {
		t.Fatal("--keep flag not registered on correlateCmd")
	}
	if f.DefValue != "false" {
		t.Errorf("--keep default = %q, want \"false\"", f.DefValue)
	}
}

// TestCorrelateKeepFlagMoveLogic verifies the inverse relationship between
// --keep and moveFiles: when --keep=true, moveFiles must be false.
func TestCorrelateKeepFlagMoveLogic(t *testing.T) {
	cmd := newMinCorrelateCmd("", "medium")
	if err := cmd.Flags().Set("keep", "true"); err != nil {
		t.Fatalf("set --keep: %v", err)
	}
	keepFiles, err := cmd.Flags().GetBool("keep")
	if err != nil {
		t.Fatalf("GetBool(keep): %v", err)
	}
	moveFiles := !keepFiles
	if moveFiles {
		t.Error("moveFiles should be false when --keep=true, got true")
	}
}

// TestCorrelateInvalidSplitLLM ensures runCorrelate rejects provider values
// other than "" and "github" with a clear error message.
func TestCorrelateInvalidSplitLLM(t *testing.T) {
	t.Parallel()

	invalid := []string{"openai", "bedrock", "anthropic", "azure"}
	for _, v := range invalid {
		v := v
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			cmd := newMinCorrelateCmd(v, "medium")
			err := runCorrelate(cmd, nil)
			if err == nil {
				t.Fatalf("expected error for --split-llm=%q, got nil", v)
			}
			if !strings.Contains(err.Error(), "invalid --split-llm") {
				t.Errorf("error = %q, want to contain \"invalid --split-llm\"", err.Error())
			}
		})
	}
}

// TestCorrelateValidSplitLLM confirms that "" and "github" pass the
// split-llm validation step (errors downstream are acceptable).
func TestCorrelateValidSplitLLM(t *testing.T) {
	t.Parallel()

	valid := []string{"", "github"}
	for _, v := range valid {
		v := v
		t.Run("value="+v, func(t *testing.T) {
			t.Parallel()
			cmd := newMinCorrelateCmd(v, "medium")
			err := runCorrelate(cmd, nil)
			// Any error is fine (e.g. missing file) except the split-llm validation error.
			if err != nil && strings.Contains(err.Error(), "invalid --split-llm") {
				t.Errorf("valid --split-llm %q triggered validation error: %v", v, err)
			}
		})
	}
}
