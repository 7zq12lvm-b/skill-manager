package skillmgr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigStoreWritesVersionTwoInstallations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewConfigStore(path)
	config := DefaultConfig()
	config.Repositories = []RepositoryConfig{{
		ID:          "github.com/example/skills",
		RepoID:      "github.com/example/skills",
		Path:        "/tmp/example-skills",
		Alias:       "Example",
		Enabled:     true,
		ScanRoots:   []string{"skills"},
		IgnorePaths: []string{"vendor"},
	}}
	if err := store.Save(config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["version"] != float64(2) {
		t.Fatalf("expected version 2 config, got %#v", raw["version"])
	}
	if _, exists := raw["repositories"]; exists {
		t.Fatal("v2 config must not persist legacy repositories")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Repositories) != 1 || loaded.Repositories[0].RepoID != "github.com/example/skills" {
		t.Fatalf("expected installation to hydrate repository scan config, got %#v", loaded.Repositories)
	}
}

func TestGitProviderRejectsFolderWithoutRepository(t *testing.T) {
	provider, ok := ProviderFor(GitProvider)
	if !ok {
		t.Fatal("git provider is not registered")
	}
	if _, _, err := provider.Inspect(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected a non-Git folder to be rejected")
	}
}
