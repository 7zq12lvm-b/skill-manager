package skillmgr

import (
	"path/filepath"
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
