package skillmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullShallowRepositoryAcrossMultipleRemoteCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	remote, writer := createGitRemote(t, root)
	checkout, _, err := cloneGitRepository(context.Background(), "file://"+remote, root, "checkout")
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 5; index++ {
		appendCommitAndPush(t, writer, index)
	}
	if _, err := pullGitRepository(context.Background(), checkout); err != nil {
		t.Fatal(err)
	}
	if value := runGit(t, checkout, "rev-parse", "--is-shallow-repository"); value != "true" {
		t.Fatalf("expected checkout to remain shallow, got %q", value)
	}
	if count := runGit(t, checkout, "rev-list", "--count", "HEAD"); count != "6" {
		t.Fatalf("expected initial commit plus five new commits, got %s", count)
	}
}

func TestPullShallowRepositoryRefusesRealLocalDivergence(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	remote, writer := createGitRemote(t, root)
	checkout, _, err := cloneGitRepository(context.Background(), "file://"+remote, root, "checkout")
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, checkout, "config", "user.email", "test@example.com")
	runGit(t, checkout, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(checkout, "local"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, checkout, "add", "local")
	runGit(t, checkout, "commit", "-m", "local commit")
	appendCommitAndPush(t, writer, 1)
	if _, err := pullGitRepository(context.Background(), checkout); err == nil || !strings.Contains(err.Error(), "Not possible to fast-forward") {
		t.Fatalf("expected local divergence to be refused, got %v", err)
	}
}

func TestValidateCloneFolderName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "/tmp/repo", `owner/repo`, `owner\repo`, "~/repo", `C:\repo`} {
		if name == "" {
			continue
		}
		if err := validateCloneFolderName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
	if err := validateCloneFolderName("mattpocock-skills"); err != nil {
		t.Fatalf("expected valid folder name, got %v", err)
	}
}

func createGitRemote(t *testing.T, root string) (string, string) {
	t.Helper()
	remote := filepath.Join(root, "remote.git")
	writer := filepath.Join(root, "writer")
	runGit(t, root, "init", "--bare", remote)
	runGit(t, root, "init", writer)
	runGit(t, writer, "config", "user.email", "test@example.com")
	runGit(t, writer, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(writer, "file"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, writer, "add", "file")
	runGit(t, writer, "commit", "-m", "initial")
	runGit(t, writer, "branch", "-M", "main")
	runGit(t, writer, "remote", "add", "origin", remote)
	runGit(t, writer, "push", "-u", "origin", "main")
	runGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	return remote, writer
}

func appendCommitAndPush(t *testing.T, writer string, index int) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(writer, "file"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(file, "%d\n", index); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	runGit(t, writer, "commit", "-am", fmt.Sprintf("commit %d", index))
	runGit(t, writer, "push", "origin", "main")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", "-C", dir)
	command.Args = append(command.Args, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
