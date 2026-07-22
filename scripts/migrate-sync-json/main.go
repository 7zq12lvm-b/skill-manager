package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	skillmgr "skill-manager/internal"
)

type migrationResult struct {
	destination string
	skills      int
	profiles    int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("migrate-sync-json", flag.ContinueOnError)
	flags.SetOutput(stderr)
	source := flags.String("source", "", "path to skill-manager-sync.json")
	destination := flags.String("destination", "", "path to skillManager.db")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: migrate-sync-json --source <skill-manager-sync.json> --destination <skillManager.db>")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || strings.TrimSpace(*source) == "" || strings.TrimSpace(*destination) == "" {
		flags.Usage()
		return 2
	}
	result, err := migrateSyncJSON(*source, *destination)
	if err != nil {
		fmt.Fprintf(stderr, "migration failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "migrated %d skills and %d profiles to %s; integrity verified; source JSON unchanged\n",
		result.skills, result.profiles, result.destination)
	return 0
}

func migrateSyncJSON(sourcePath, destinationPath string) (migrationResult, error) {
	result := migrationResult{}
	sourcePath = filepath.Clean(sourcePath)
	destinationPath = filepath.Clean(destinationPath)
	if filepath.Base(destinationPath) != skillmgr.SyncFileName {
		return result, fmt.Errorf("destination filename must be %s", skillmgr.SyncFileName)
	}
	sourceInfo, err := os.Lstat(sourcePath)
	if err != nil {
		return result, fmt.Errorf("inspect source JSON: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return result, errors.New("source JSON must be a regular file")
	}
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return result, fmt.Errorf("read source JSON: %w", err)
	}
	document, err := decodeSyncDocument(sourceBytes)
	if err != nil {
		return result, err
	}
	destinationDir := filepath.Dir(destinationPath)
	destinationInfo, err := os.Stat(destinationDir)
	if err != nil {
		return result, fmt.Errorf("inspect destination directory: %w", err)
	}
	if !destinationInfo.IsDir() {
		return result, errors.New("destination parent must be a directory")
	}
	if err := requireMissing(destinationPath); err != nil {
		return result, err
	}
	tempDir, err := os.MkdirTemp(destinationDir, ".skill-manager-migrate-*")
	if err != nil {
		return result, fmt.Errorf("create migration workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)
	tempDatabasePath := filepath.Join(tempDir, skillmgr.SyncFileName)
	store := skillmgr.NewSyncStore(tempDatabasePath)
	if err := store.Save(document); err != nil {
		return result, fmt.Errorf("write temporary database: %w", err)
	}
	if err := store.CheckIntegrity(); err != nil {
		return result, fmt.Errorf("validate temporary database: %w", err)
	}
	loaded, err := store.Load()
	if err != nil {
		return result, fmt.Errorf("read temporary database: %w", err)
	}
	if err := compareSyncDocuments(document, loaded); err != nil {
		return result, err
	}
	if err := requireNoSidecars(tempDatabasePath); err != nil {
		return result, err
	}
	currentSource, err := os.ReadFile(sourcePath)
	if err != nil {
		return result, fmt.Errorf("re-read source JSON: %w", err)
	}
	if !bytes.Equal(currentSource, sourceBytes) {
		return result, errors.New("source JSON changed during migration")
	}
	if err := requireMissing(destinationPath); err != nil {
		return result, err
	}
	if err := publishDatabaseNoReplace(tempDatabasePath, destinationPath); err != nil {
		return result, err
	}
	return migrationResult{
		destination: destinationPath,
		skills:      len(loaded.Skills),
		profiles:    len(loaded.Profiles),
	}, nil
}

func publishDatabaseNoReplace(temporaryPath, destinationPath string) error {
	if err := os.Link(temporaryPath, destinationPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("destination already exists")
		}
		return fmt.Errorf("publish database: %w", err)
	}
	return nil
}

func decodeSyncDocument(data []byte) (skillmgr.SyncDocument, error) {
	var document skillmgr.SyncDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return document, fmt.Errorf("decode source JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return document, errors.New("decode source JSON: unexpected trailing value")
		}
		return document, fmt.Errorf("decode source JSON: %w", err)
	}
	if document.Version != 2 {
		return document, errors.New("source JSON version must be 2")
	}
	return document, nil
}

func requireMissing(path string) error {
	_, err := os.Lstat(path)
	if err == nil {
		return errors.New("destination already exists")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect destination: %w", err)
	}
	return nil
}

func requireNoSidecars(databasePath string) error {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if _, err := os.Lstat(databasePath + suffix); err == nil {
			return fmt.Errorf("temporary database left a %s sidecar", suffix)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect temporary database sidecar: %w", err)
		}
	}
	return nil
}

func compareSyncDocuments(source, database skillmgr.SyncDocument) error {
	if database.Version != source.Version {
		return comparisonError("version")
	}
	if source.LLM.BaseURL != database.LLM.BaseURL {
		return comparisonError("llm.baseUrl")
	}
	if source.LLM.APIKey != database.LLM.APIKey {
		return comparisonError("llm.apiKey")
	}
	if source.LLM.Model != database.LLM.Model {
		return comparisonError("llm.model")
	}
	if source.LLM.Temperature != database.LLM.Temperature {
		return comparisonError("llm.temperature")
	}
	if source.LLM.MaxTokens != database.LLM.MaxTokens {
		return comparisonError("llm.maxTokens")
	}
	expectedProfiles := effectiveProfiles(source)
	if len(expectedProfiles) != len(database.Profiles) {
		return comparisonError("profiles")
	}
	for syncID, expected := range expectedProfiles {
		actual, ok := database.Profiles[syncID]
		if !ok {
			return comparisonError(fmt.Sprintf("profiles[%q]", syncID))
		}
		if field := compareProfiles(expected, actual); field != "" {
			return comparisonError(fmt.Sprintf("profiles[%q].%s", syncID, field))
		}
	}
	if len(source.Skills) != len(database.Skills) {
		return comparisonError("skills")
	}
	for syncID, expected := range source.Skills {
		actual, ok := database.Skills[syncID]
		if !ok {
			return comparisonError(fmt.Sprintf("skills[%q]", syncID))
		}
		if field := compareSkillRecords(expected, actual); field != "" {
			return comparisonError(fmt.Sprintf("skills[%q].%s", syncID, field))
		}
		expectedProfile, hasProfile := expectedProfiles[syncID]
		if !hasProfile {
			if actual.Profile != nil {
				return comparisonError(fmt.Sprintf("skills[%q].profile", syncID))
			}
			continue
		}
		if actual.Profile == nil {
			return comparisonError(fmt.Sprintf("skills[%q].profile", syncID))
		}
		if field := compareProfiles(expectedProfile, *actual.Profile); field != "" {
			return comparisonError(fmt.Sprintf("skills[%q].profile.%s", syncID, field))
		}
	}
	return nil
}

func effectiveProfiles(document skillmgr.SyncDocument) map[string]skillmgr.SkillProfile {
	profiles := make(map[string]skillmgr.SkillProfile, len(document.Skills))
	for syncID, record := range document.Skills {
		if profile, exists := document.Profiles[syncID]; exists {
			profiles[syncID] = profile
		} else if record.Profile != nil {
			profiles[syncID] = *record.Profile
		}
	}
	return profiles
}

func compareSkillRecords(expected, actual skillmgr.SyncSkillRecord) string {
	switch {
	case expected.Enabled != actual.Enabled:
		return "enabled"
	case expected.TargetName != actual.TargetName:
		return "targetName"
	case !equalStringSlices(expected.PreviousTargetNames, actual.PreviousTargetNames):
		return "previousTargetNames"
	case !equalStringSlices(expected.Tags, actual.Tags):
		return "tags"
	case expected.Note != actual.Note:
		return "note"
	case expected.UpdatedAt != actual.UpdatedAt:
		return "updatedAt"
	case expected.Source.Provider != actual.Source.Provider:
		return "source.provider"
	case expected.Source.ID != actual.Source.ID:
		return "source.id"
	case expected.Source.Locator.CloneURL != actual.Source.Locator.CloneURL:
		return "source.locator.cloneUrl"
	case expected.Source.Locator.Subpath != actual.Source.Locator.Subpath:
		return "source.locator.subpath"
	case expected.Source.Locator.Ref != actual.Source.Locator.Ref:
		return "source.locator.ref"
	default:
		return ""
	}
}

func compareProfiles(expected, actual skillmgr.SkillProfile) string {
	switch {
	case expected.SummaryZh != actual.SummaryZh:
		return "summaryZh"
	case !equalStringSlices(expected.UseCasesZh, actual.UseCasesZh):
		return "useCasesZh"
	case expected.GeneratedAt != actual.GeneratedAt:
		return "generatedAt"
	case expected.Model != actual.Model:
		return "model"
	case expected.SourceHash != actual.SourceHash:
		return "sourceHash"
	case expected.Error != actual.Error:
		return "error"
	default:
		return ""
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func comparisonError(path string) error {
	return fmt.Errorf("migration verification failed at %s", path)
}
