package skillmgr

type ValidationMode string

const (
	ValidationLoose  ValidationMode = "loose"
	ValidationStrict ValidationMode = "strict"
	ValidationCustom ValidationMode = "custom"
)

type SkillStatus string

const (
	StatusEnabled       SkillStatus = "enabled"
	StatusDisabled      SkillStatus = "disabled"
	StatusConflict      SkillStatus = "conflict"
	StatusInvalid       SkillStatus = "invalid"
	StatusMissingSource SkillStatus = "missing-source"
	StatusMissingPath   SkillStatus = "missing-path"
	StatusError         SkillStatus = "error"
)

type Config struct {
	Version          int                  `json:"version"`
	TargetDirs       []string             `json:"targetDirs"`
	Installations    []SourceInstallation `json:"installations"`
	Sources          []SkillSourceConfig  `json:"-"`
	Repositories     []RepositoryConfig   `json:"-"`
	Validation       ValidationConfig     `json:"validation"`
	Scan             ScanConfig           `json:"scan"`
	Sync             SyncConfig           `json:"sync"`
	ConflictHandling string               `json:"conflictHandling"`
	SourcePriority   []string             `json:"sourcePriority"`
}

type SourceInstallation struct {
	Provider string                    `json:"provider"`
	SourceID string                    `json:"sourceId"`
	Path     string                    `json:"path"`
	Alias    string                    `json:"alias,omitempty"`
	Enabled  bool                      `json:"enabled"`
	Options  SourceInstallationOptions `json:"options,omitempty"`
}

type SourceInstallationOptions struct {
	ScanRoots   []string `json:"scanRoots,omitempty"`
	IgnorePaths []string `json:"ignorePaths,omitempty"`
}

type SkillSourceConfig struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Alias   string `json:"alias,omitempty"`
	Enabled bool   `json:"enabled"`
}

type RepositoryConfig struct {
	ID          string   `json:"id"`
	RepoID      string   `json:"repoId"`
	Path        string   `json:"path"`
	Alias       string   `json:"alias,omitempty"`
	Enabled     bool     `json:"enabled"`
	CloneURL    string   `json:"cloneUrl,omitempty"`
	ScanRoots   []string `json:"scanRoots,omitempty"`
	IgnorePaths []string `json:"ignorePaths,omitempty"`
}

type SyncConfig struct {
	Folder        string `json:"folder,omitempty"`
	LastAppliedAt string `json:"lastAppliedAt,omitempty"`
}

type ValidationConfig struct {
	Mode          ValidationMode `json:"mode"`
	RequiredFiles []string       `json:"requiredFiles"`
	ShowInvalid   bool           `json:"showInvalid"`
}

type ScanConfig struct {
	AutoRescanOnStartup bool `json:"autoRescanOnStartup"`
	WatchSourceFolders  bool `json:"watchSourceFolders"`
}

type Inventory struct {
	Config         Config        `json:"config"`
	Sources        []SkillSource `json:"sources"`
	Repositories   []Repository  `json:"repositories,omitempty"`
	Skills         []Skill       `json:"skills"`
	Summary        Summary       `json:"summary"`
	SyncConfigured bool          `json:"syncConfigured"`
	SyncPath       string        `json:"syncPath,omitempty"`
	SyncError      string        `json:"syncError,omitempty"`
	LLMConfig      SyncLLMConfig `json:"llmConfig,omitempty"`
}

type PullSourceResult struct {
	Inventory Inventory `json:"inventory"`
	Message   string    `json:"message"`
}

type BulkEnableResult struct {
	Inventory      Inventory `json:"inventory"`
	Enabled        int       `json:"enabled"`
	AlreadyEnabled int       `json:"alreadyEnabled"`
	Skipped        int       `json:"skipped"`
	Failed         []string  `json:"failed,omitempty"`
}

type CloneRepositoryResult struct {
	Inventory Inventory `json:"inventory"`
	Message   string    `json:"message"`
}

type SkillProfileResult struct {
	Inventory Inventory     `json:"inventory"`
	Profile   *SkillProfile `json:"profile,omitempty"`
	Generated bool          `json:"generated"`
	Message   string        `json:"message,omitempty"`
}

type SkillSource struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	Alias         string `json:"alias,omitempty"`
	Enabled       bool   `json:"enabled"`
	IsGitRepo     bool   `json:"isGitRepo"`
	GitRoot       string `json:"gitRoot,omitempty"`
	SkillCount    int    `json:"skillCount"`
	LastScannedAt string `json:"lastScannedAt,omitempty"`
	ErrorCount    int    `json:"errorCount"`
	Error         string `json:"error,omitempty"`
}

type Repository struct {
	ID            string   `json:"id"`
	Provider      string   `json:"provider"`
	SourceKey     string   `json:"sourceKey"`
	RepoID        string   `json:"repoId"`
	Path          string   `json:"path"`
	Alias         string   `json:"alias,omitempty"`
	Enabled       bool     `json:"enabled"`
	CloneURL      string   `json:"cloneUrl,omitempty"`
	ScanRoots     []string `json:"scanRoots,omitempty"`
	IgnorePaths   []string `json:"ignorePaths,omitempty"`
	SkillCount    int      `json:"skillCount"`
	Installed     bool     `json:"installed"`
	LastScannedAt string   `json:"lastScannedAt,omitempty"`
	IsGitRepo     bool     `json:"isGitRepo"`
	CurrentRef    string   `json:"currentRef,omitempty"`
	Dirty         bool     `json:"dirty"`
	ErrorCount    int      `json:"errorCount"`
	Error         string   `json:"error,omitempty"`
}

type Skill struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	DisplayName         string           `json:"displayName,omitempty"`
	SourceID            string           `json:"sourceId"`
	SourceKey           string           `json:"sourceKey,omitempty"`
	SourceAlias         string           `json:"sourceAlias,omitempty"`
	SourcePath          string           `json:"sourcePath"`
	RepoID              string           `json:"repoId,omitempty"`
	RepoPath            string           `json:"repoPath,omitempty"`
	RepoSubpath         string           `json:"repoSubpath,omitempty"`
	CloneURL            string           `json:"cloneUrl,omitempty"`
	SyncID              string           `json:"syncId,omitempty"`
	TargetName          string           `json:"targetName,omitempty"`
	PreviousTargetNames []string         `json:"previousTargetNames,omitempty"`
	TargetPath          string           `json:"targetPath,omitempty"`
	SymlinkPath         string           `json:"symlinkPath,omitempty"`
	TargetStates        []SkillTarget    `json:"targetStates,omitempty"`
	Status              SkillStatus      `json:"status"`
	HasSymlink          bool             `json:"hasSymlink"`
	SymlinkTarget       string           `json:"symlinkTarget,omitempty"`
	IsActive            bool             `json:"isActive"`
	IsSynced            bool             `json:"-"`
	DesiredEnabled      *bool            `json:"-"`
	CanSync             bool             `json:"-"`
	Ref                 string           `json:"ref,omitempty"`
	RefMismatch         bool             `json:"refMismatch"`
	ValidationErrors    []string         `json:"validationErrors,omitempty"`
	Files               []string         `json:"files,omitempty"`
	Description         string           `json:"description,omitempty"`
	Manifest            *SkillManifest   `json:"manifest,omitempty"`
	PreviewFile         string           `json:"previewFile,omitempty"`
	Preview             string           `json:"preview,omitempty"`
	UpdatedAt           string           `json:"updatedAt,omitempty"`
	LastScannedAt       string           `json:"lastScannedAt,omitempty"`
	ConflictSources     []ConflictSource `json:"conflictSources,omitempty"`
	Tags                []string         `json:"tags,omitempty"`
	Profile             *SkillProfile    `json:"profile,omitempty"`
	Error               string           `json:"error,omitempty"`
}

type SkillFileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

type SkillProfile struct {
	SummaryZh   string   `json:"summaryZh,omitempty"`
	UseCasesZh  []string `json:"useCasesZh,omitempty"`
	GeneratedAt string   `json:"generatedAt,omitempty"`
	Model       string   `json:"model,omitempty"`
	SourceHash  string   `json:"sourceHash,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type SkillTarget struct {
	TargetDir     string `json:"targetDir"`
	TargetPath    string `json:"targetPath"`
	SymlinkPath   string `json:"symlinkPath"`
	HasSymlink    bool   `json:"hasSymlink"`
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
	IsActive      bool   `json:"isActive"`
	Error         string `json:"error,omitempty"`
}

type SkillManifest struct {
	Name                   string            `json:"name,omitempty"`
	Description            string            `json:"description,omitempty"`
	License                string            `json:"license,omitempty"`
	Compatibility          string            `json:"compatibility,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
	AllowedTools           string            `json:"allowedTools,omitempty"`
	WhenToUse              string            `json:"whenToUse,omitempty"`
	DisableModelInvocation *bool             `json:"disableModelInvocation,omitempty"`
	UserInvocable          *bool             `json:"userInvocable,omitempty"`
	ArgumentHint           string            `json:"argumentHint,omitempty"`
	Arguments              any               `json:"arguments,omitempty"`
}

type ConflictSource struct {
	SkillID    string      `json:"skillId"`
	SourceID   string      `json:"sourceId"`
	SourcePath string      `json:"sourcePath"`
	Status     SkillStatus `json:"status"`
}

type Summary struct {
	SkillsFound int `json:"skillsFound"`
	Enabled     int `json:"enabled"`
	Conflicts   int `json:"conflicts"`
	Invalid     int `json:"invalid"`
	Errors      int `json:"errors"`
}

func DefaultConfig() Config {
	return Config{
		Version:    2,
		TargetDirs: []string{expandHome("~/.agents/skills")},
		Validation: ValidationConfig{
			Mode:          ValidationStrict,
			RequiredFiles: []string{"SKILL.md"},
			ShowInvalid:   false,
		},
		Scan: ScanConfig{
			WatchSourceFolders: true,
		},
		ConflictHandling: "ask",
	}
}

func NewSkillSourceConfig(path string) SkillSourceConfig {
	path = expandHome(path)
	return SkillSourceConfig{
		ID:      sourceID(path),
		Path:    path,
		Enabled: true,
	}
}
