package skillmgr

type ValidationMode string

const (
	ValidationLoose  ValidationMode = "loose"
	ValidationStrict ValidationMode = "strict"
	ValidationCustom ValidationMode = "custom"
)

type SkillStatus string

const (
	StatusSynced   SkillStatus = "synced"
	StatusDisabled SkillStatus = "disabled"
	StatusConflict SkillStatus = "conflict"
	StatusInvalid  SkillStatus = "invalid"
	StatusMissing  SkillStatus = "missing"
	StatusSyncing  SkillStatus = "syncing"
	StatusError    SkillStatus = "error"
)

type Config struct {
	TargetDir        string              `json:"targetDir"`
	Sources          []SkillSourceConfig `json:"sources"`
	Validation       ValidationConfig    `json:"validation"`
	Scan             ScanConfig          `json:"scan"`
	ConflictHandling string              `json:"conflictHandling"`
	SourcePriority   []string            `json:"sourcePriority"`
}

type SkillSourceConfig struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Alias   string `json:"alias,omitempty"`
	Enabled bool   `json:"enabled"`
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
	Config  Config        `json:"config"`
	Sources []SkillSource `json:"sources"`
	Skills  []Skill       `json:"skills"`
	Summary Summary       `json:"summary"`
}

type SkillSource struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	Alias         string `json:"alias,omitempty"`
	Enabled       bool   `json:"enabled"`
	SkillCount    int    `json:"skillCount"`
	LastScannedAt string `json:"lastScannedAt,omitempty"`
	ErrorCount    int    `json:"errorCount"`
	Error         string `json:"error,omitempty"`
}

type Skill struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	SourceID         string           `json:"sourceId"`
	SourceAlias      string           `json:"sourceAlias,omitempty"`
	SourcePath       string           `json:"sourcePath"`
	TargetPath       string           `json:"targetPath"`
	SymlinkPath      string           `json:"symlinkPath"`
	Status           SkillStatus      `json:"status"`
	HasSymlink       bool             `json:"hasSymlink"`
	SymlinkTarget    string           `json:"symlinkTarget,omitempty"`
	ValidationErrors []string         `json:"validationErrors,omitempty"`
	Files            []string         `json:"files,omitempty"`
	Description      string           `json:"description,omitempty"`
	PreviewFile      string           `json:"previewFile,omitempty"`
	Preview          string           `json:"preview,omitempty"`
	UpdatedAt        string           `json:"updatedAt,omitempty"`
	LastScannedAt    string           `json:"lastScannedAt,omitempty"`
	ConflictSources  []ConflictSource `json:"conflictSources,omitempty"`
	Error            string           `json:"error,omitempty"`
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
		TargetDir: expandHome("~/.agents/skills"),
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
