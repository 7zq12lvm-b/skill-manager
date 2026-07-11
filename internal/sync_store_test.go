package skillmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncStoreSavesLLMConfigAndSkillProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	store := NewSyncStore(path)
	syncID := "git:example.com/me/repo//skills/code-review"

	err := store.Save(SyncDocument{
		Version: 2,
		Skills: map[string]SyncSkillRecord{
			syncID: {
				Enabled:    true,
				TargetName: "code-review",
				Source:     SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/code-review"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLLMConfig(SyncLLMConfig{
		BaseURL:     " https://api.deepseek.com ",
		APIKey:      " secret ",
		Model:       " deepseek-v4-flash ",
		Temperature: -1,
		MaxTokens:   -100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSkillProfile(syncID, SkillProfile{
		SummaryZh:  "  代码审阅助手。 ",
		UseCasesZh: []string{"指出回归风险。", ""},
		Model:      " deepseek-v4-flash ",
	}); err != nil {
		t.Fatal(err)
	}

	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if document.LLM.BaseURL != "https://api.deepseek.com" || document.LLM.APIKey != "secret" ||
		document.LLM.Model != "deepseek-v4-flash" || document.LLM.Temperature != 0 || document.LLM.MaxTokens != 0 {
		t.Fatalf("unexpected LLM config: %#v", document.LLM)
	}
	profile, ok := document.Profiles[syncID]
	if !ok {
		t.Fatalf("expected top-level profile for %s", syncID)
	}
	if profile.SummaryZh != "代码审阅助手。" || len(profile.UseCasesZh) != 1 {
		t.Fatalf("unexpected profile: %#v", profile)
	}
	if document.Skills[syncID].Profile == nil || document.Skills[syncID].Profile.SummaryZh != "代码审阅助手。" {
		t.Fatalf("expected synced skill record to mirror profile, got %#v", document.Skills[syncID].Profile)
	}
}

func TestSyncStoreUpsertSkillsWritesMultipleRecords(t *testing.T) {
	store := NewSyncStore(filepath.Join(t.TempDir(), SyncFileName))
	records := []SyncSkillRecord{
		{
			Enabled: true,
			Tags:    []string{" review ", "review"},
			Source:  SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
		},
		{
			Tags:   []string{"writing"},
			Source: SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/write"}},
		},
	}
	if err := store.UpsertSkills(records); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Skills) != 2 {
		t.Fatalf("expected two records, got %#v", document.Skills)
	}
	for id, record := range document.Skills {
		if record.UpdatedAt == "" {
			t.Fatalf("expected %s to have a shared update timestamp", id)
		}
	}
	if tags := document.Skills["git:example.com/me/repo//skills/review"].Tags; len(tags) != 1 || tags[0] != "review" {
		t.Fatalf("expected normalized tags, got %#v", tags)
	}
}

func TestSyncStoreNormalizesAndClearsSkillNote(t *testing.T) {
	path := filepath.Join(t.TempDir(), SyncFileName)
	store := NewSyncStore(path)
	record := SyncSkillRecord{
		Note:   "  first line\nsecond line  ",
		Source: SyncSource{Provider: GitProvider, ID: "example.com/me/repo", Locator: SourceLocator{Subpath: "skills/review"}},
	}
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	document, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	syncID := "git:example.com/me/repo//skills/review"
	if note := document.Skills[syncID].Note; note != "first line\nsecond line" {
		t.Fatalf("expected normalized multiline note, got %q", note)
	}
	record.Note = " \n\t "
	if err := store.UpsertSkill(record); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"note"`) {
		t.Fatalf("expected empty note to be omitted, got %s", data)
	}
}
