package skillmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func gitRepositoryRoot(ctx context.Context, path string) (string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false
	}
	output, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(output))
	return root, root != ""
}

func pullGitRepository(ctx context.Context, path string) (string, error) {
	if path == "" {
		return "", errors.New("source path is required")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", errors.New("git command not found")
	}
	if _, ok := gitRepositoryRoot(ctx, path); !ok {
		return "", fmt.Errorf("source path is not inside a git repository: %s", path)
	}

	cmd := exec.CommandContext(ctx, "git", "-C", path, "pull")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git pull failed: %s", message)
	}
	message := strings.TrimSpace(output.String())
	if message == "" {
		message = "Pull completed."
	}
	return message, nil
}
