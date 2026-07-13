package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	skillmgr "skill-manager/internal"
)

func TestRunMigratesSyncJSONWithoutChangingSource(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "skill-manager-sync.json")
	destinationPath := filepath.Join(root, skillmgr.SyncFileName)
	syncID := "git:example.com/me/repo//skills/review"
	profile := skillmgr.SkillProfile{
		SummaryZh:   "代码审阅助手。",
		UseCasesZh:  []string{"发现回归。", "解释修改。"},
		GeneratedAt: "2026-07-13T08:00:00Z",
		Model:       "model-v1",
		SourceHash:  "source-hash",
	}
	document := skillmgr.SyncDocument{
		Version: 2,
		LLM: skillmgr.SyncLLMConfig{
			BaseURL:     "https://api.example.com",
			APIKey:      "migration-secret",
			Model:       "model-v1",
			Temperature: 0.2,
			MaxTokens:   4096,
		},
		Profiles: map[string]skillmgr.SkillProfile{
			syncID:   profile,
			"orphan": {SummaryZh: "孤立简介。", UseCasesZh: []string{"仍需保留。"}},
		},
		Skills: map[string]skillmgr.SyncSkillRecord{
			syncID: {
				Enabled:             true,
				TargetName:          "review",
				PreviousTargetNames: []string{"old-review"},
				Tags:                []string{"quality", "review"},
				Note:                "personal note",
				Profile:             &profile,
				UpdatedAt:           "2026-07-13T08:01:00Z",
				Source: skillmgr.SyncSource{
					Provider: skillmgr.GitProvider,
					ID:       "example.com/me/repo",
					Locator: skillmgr.SourceLocator{
						CloneURL: "https://example.com/me/repo.git",
						Subpath:  "skills/review",
						Ref:      "main",
					},
				},
			},
		},
	}
	sourceBytes, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--source", sourcePath, "--destination", destinationPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("expected successful migration, code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), document.LLM.APIKey) || strings.Contains(stderr.String(), document.LLM.APIKey) {
		t.Fatal("migration output exposed the API key")
	}
	afterSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterSource, sourceBytes) {
		t.Fatal("expected source JSON to remain byte-for-byte unchanged")
	}
	loaded, err := skillmgr.NewSyncStore(destinationPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Skills) != 1 || len(loaded.Profiles) != 2 {
		t.Fatalf("unexpected migrated counts: skills=%d profiles=%d", len(loaded.Skills), len(loaded.Profiles))
	}
	record := loaded.Skills[syncID]
	if !record.Enabled || record.Note != "personal note" || record.Source.Locator.Ref != "main" || len(record.Tags) != 2 {
		t.Fatalf("unexpected migrated skill: %#v", record)
	}
	if got := loaded.Profiles[syncID].UseCasesZh; len(got) != 2 || got[0] != "发现回归。" || got[1] != "解释修改。" {
		t.Fatalf("unexpected migrated profile use cases: %#v", got)
	}
	if orphan, ok := loaded.Profiles["orphan"]; !ok || orphan.SummaryZh != "孤立简介。" {
		t.Fatalf("expected orphan profile to survive, got %#v", orphan)
	}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if _, err := os.Lstat(destinationPath + suffix); !os.IsNotExist(err) {
			t.Fatalf("expected no SQLite sidecar %s, got %v", suffix, err)
		}
	}
}

func TestRunRefusesToOverwriteExistingDestination(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "skill-manager-sync.json")
	destinationPath := filepath.Join(root, skillmgr.SyncFileName)
	sourceBytes := []byte(`{"version":2,"skills":{}}`)
	destinationBytes := []byte("existing database placeholder")
	if err := os.WriteFile(sourcePath, sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, destinationBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--source", sourcePath, "--destination", destinationPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("expected operational failure, code=%d stderr=%s", code, stderr.String())
	}
	afterDestination, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterDestination, destinationBytes) {
		t.Fatal("existing destination was modified")
	}
}

func TestRunRejectsInvalidJSONWithoutPublishingDatabase(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "skill-manager-sync.json")
	destinationPath := filepath.Join(root, skillmgr.SyncFileName)
	secret := "invalid-json-secret"
	sourceBytes := []byte(`{"version":2,"llm":{"apiKey":"` + secret + `"},"skills":{},"unexpected":true}`)
	if err := os.WriteFile(sourcePath, sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--source", sourcePath, "--destination", destinationPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("expected operational failure, code=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatal("migration error output exposed the API key")
	}
	if _, err := os.Lstat(destinationPath); !os.IsNotExist(err) {
		t.Fatalf("expected no destination database, got %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(sourcePath) {
		t.Fatalf("expected only the source JSON to remain, got %#v", entries)
	}
}

func TestPublishDatabaseNoReplacePreservesRacingDestination(t *testing.T) {
	root := t.TempDir()
	temporaryPath := filepath.Join(root, "temporary.db")
	destinationPath := filepath.Join(root, skillmgr.SyncFileName)
	temporaryBytes := []byte("validated temporary database")
	destinationBytes := []byte("destination created by another process")
	if err := os.WriteFile(temporaryPath, temporaryBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, destinationBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishDatabaseNoReplace(temporaryPath, destinationPath); err == nil {
		t.Fatal("expected atomic publication to reject an existing destination")
	}
	afterDestination, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterDestination, destinationBytes) {
		t.Fatal("racing destination was overwritten")
	}
	afterTemporary, err := os.ReadFile(temporaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterTemporary, temporaryBytes) {
		t.Fatal("temporary database was modified after failed publication")
	}
}
