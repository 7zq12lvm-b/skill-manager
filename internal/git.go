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
	upstream, err := currentGitUpstream(ctx, path)
	if err != nil {
		return "", err
	}
	fetchSpec := "+" + upstream.mergeRef + ":refs/remotes/" + upstream.remote + "/" + upstream.branch
	fetchOutput, fetchErr := runGitCombined(ctx, path, "fetch", "--no-tags", upstream.remote, fetchSpec)
	if fetchErr != nil {
		return "", fmt.Errorf("git fetch failed: %s", gitFailureMessage(fetchOutput, fetchErr))
	}
	localHead, err := gitOutput(ctx, path, "rev-parse", "HEAD")
	if err != nil {
		return "", errors.New("could not resolve the local repository head")
	}
	upstreamHead, err := gitOutput(ctx, path, "rev-parse", upstream.ref)
	if err != nil {
		return "", fmt.Errorf("could not resolve upstream branch %s", upstream.ref)
	}
	if localHead == upstreamHead {
		return "Already up to date.", nil
	}
	recoveredShallowHistory := false
	if !gitIsAncestor(ctx, path, localHead, upstreamHead) && !gitHasMergeBase(ctx, path, localHead, upstreamHead) && gitIsShallow(ctx, path) {
		deepenOutput, deepenErr := runGitCombined(ctx, path, "fetch", "--deepen=50", "--no-tags", upstream.remote, fetchSpec)
		if deepenErr != nil {
			return "", fmt.Errorf("git fetch failed while deepening shallow history: %s", gitFailureMessage(deepenOutput, deepenErr))
		}
		recoveredShallowHistory = true
	}
	if !gitIsAncestor(ctx, path, localHead, upstreamHead) {
		return "", errors.New("git pull failed: Not possible to fast-forward because the local and upstream branches have diverged")
	}
	resetOutput, resetErr := runGitCombined(ctx, path, "reset", "--merge", upstream.ref)
	if resetErr != nil {
		return "", fmt.Errorf("git fast-forward blocked by local changes in upstream-modified files: %s", gitFailureMessage(resetOutput, resetErr))
	}
	if recoveredShallowHistory {
		return "Recovered shallow history and fast-forwarded to " + upstream.ref + ".", nil
	}
	return "Fast-forwarded to " + upstream.ref + ".", nil
}

type gitUpstream struct {
	remote   string
	branch   string
	mergeRef string
	ref      string
}

func currentGitUpstream(ctx context.Context, path string) (gitUpstream, error) {
	currentBranch, err := gitOutput(ctx, path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || currentBranch == "" {
		return gitUpstream{}, errors.New("git pull requires a checked-out branch")
	}
	remote, err := gitOutput(ctx, path, "config", "--get", "branch."+currentBranch+".remote")
	if err != nil || remote == "" || remote == "." {
		return gitUpstream{}, fmt.Errorf("git branch %s does not have a remote upstream", currentBranch)
	}
	mergeRef, err := gitOutput(ctx, path, "config", "--get", "branch."+currentBranch+".merge")
	if err != nil || !strings.HasPrefix(mergeRef, "refs/heads/") {
		return gitUpstream{}, fmt.Errorf("git branch %s does not have an upstream branch", currentBranch)
	}
	branch := strings.TrimPrefix(mergeRef, "refs/heads/")
	return gitUpstream{
		remote:   remote,
		branch:   branch,
		mergeRef: mergeRef,
		ref:      remote + "/" + branch,
	}, nil
}

func gitIsAncestor(ctx context.Context, path, ancestor, descendant string) bool {
	return exec.CommandContext(ctx, "git", "-C", path, "merge-base", "--is-ancestor", ancestor, descendant).Run() == nil
}

func gitHasMergeBase(ctx context.Context, path, left, right string) bool {
	_, err := gitOutput(ctx, path, "merge-base", left, right)
	return err == nil
}

func gitIsShallow(ctx context.Context, path string) bool {
	value, err := gitOutput(ctx, path, "rev-parse", "--is-shallow-repository")
	return err == nil && value == "true"
}

func runGitCombined(ctx context.Context, path string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", path}, args...)
	cmd := exec.CommandContext(ctx, "git", commandArgs...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return strings.TrimSpace(output.String()), err
}

func gitOutput(ctx context.Context, path string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", path}, args...)
	output, err := exec.CommandContext(ctx, "git", commandArgs...).Output()
	return strings.TrimSpace(string(output)), err
}

func gitFailureMessage(message string, err error) string {
	if message != "" {
		return message
	}
	return err.Error()
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
