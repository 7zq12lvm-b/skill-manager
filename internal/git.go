package skillmgr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
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

func GitRepositoryRootForApp(ctx context.Context, path string) (string, bool) {
	return gitRepositoryRoot(ctx, path)
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
	args := []string{"-C", path, "pull", "--ff-only"}
	cmd := exec.CommandContext(ctx, "git", args...)
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

func cloneGitRepository(ctx context.Context, cloneURL, parentDir, folderName string) (string, string, error) {
	if strings.TrimSpace(cloneURL) == "" {
		return "", "", errors.New("clone URL is required")
	}
	if strings.TrimSpace(parentDir) == "" {
		return "", "", errors.New("parent folder is required")
	}
	if strings.TrimSpace(folderName) == "" {
		return "", "", errors.New("folder name is required")
	}
	folderName = strings.TrimSpace(folderName)
	if err := validateCloneFolderName(folderName); err != nil {
		return "", "", err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", "", errors.New("git command not found")
	}
	targetPath := filepath.Join(parentDir, folderName)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", "--single-branch", "--no-tags", cloneURL, targetPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			message = err.Error()
		}
		return "", "", fmt.Errorf("git clone failed: %s", message)
	}
	message := strings.TrimSpace(output.String())
	if message == "" {
		message = "Clone completed."
	}
	return targetPath, message, nil
}

func validateCloneFolderName(folderName string) error {
	if folderName == "." || folderName == ".." {
		return errors.New("folder name must not be . or ..")
	}
	if filepath.IsAbs(folderName) || strings.ContainsAny(folderName, `/\`) || strings.HasPrefix(folderName, "~") {
		return errors.New("folder name must be a single relative directory name")
	}
	if len(folderName) >= 2 && folderName[1] == ':' {
		return errors.New("folder name must not be an absolute Windows path")
	}
	return nil
}

func gitRemoteURL(ctx context.Context, path string) (string, bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", false
	}
	output, err := exec.CommandContext(ctx, "git", "-C", path, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", false
	}
	remote := strings.TrimSpace(string(output))
	return remote, remote != ""
}

func GitRemoteURLForApp(ctx context.Context, path string) (string, bool) {
	return gitRemoteURL(ctx, path)
}

func gitCurrentRef(ctx context.Context, path string) string {
	if _, err := exec.LookPath("git"); err != nil {
		return ""
	}
	output, err := exec.CommandContext(ctx, "git", "-C", path, "branch", "--show-current").Output()
	if err == nil {
		ref := strings.TrimSpace(string(output))
		if ref != "" {
			return ref
		}
	}
	output, err = exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

var scpLikeGitRemote = regexp.MustCompile(`^(?:[^@]+@)?([^:]+):/?(.+)$`)

func canonicalGitRemote(remote string) (string, bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", false
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Host)
		path := strings.Trim(parsed.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		if host != "" && path != "" {
			return host + "/" + path, true
		}
	}
	if matches := scpLikeGitRemote.FindStringSubmatch(remote); len(matches) == 3 {
		host := strings.ToLower(strings.TrimSpace(matches[1]))
		path := strings.Trim(strings.TrimSpace(matches[2]), "/")
		path = strings.TrimSuffix(path, ".git")
		if host != "" && path != "" {
			return host + "/" + path, true
		}
	}
	path := strings.Trim(remote, "/")
	path = strings.TrimSuffix(path, ".git")
	if strings.Count(path, "/") >= 2 && !strings.Contains(path, " ") {
		parts := strings.Split(path, "/")
		parts[0] = strings.ToLower(parts[0])
		return strings.Join(parts, "/"), true
	}
	return "", false
}

func CanonicalGitRemoteForApp(remote string) (string, bool) {
	return canonicalGitRemote(remote)
}

func syncSkillID(repoID, repoSubpath string) string {
	repoID = strings.Trim(strings.TrimSpace(repoID), "/")
	repoSubpath = cleanRepoSubpath(repoSubpath)
	if repoID == "" || repoSubpath == "" {
		return ""
	}
	return "git:" + repoID + "//" + repoSubpath
}

func cleanRepoSubpath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "."
	}
	path = filepath.Clean(path)
	path = filepath.ToSlash(path)
	return strings.Trim(path, "/")
}
