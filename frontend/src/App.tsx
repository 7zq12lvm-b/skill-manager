import { type CSSProperties, type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import {
  AlertTriangle,
  Check,
  ChevronDown,
  ChevronRight,
  Circle,
  CloudDownload,
  ExternalLink,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  Loader2,
  List,
  PanelLeftOpen,
  Plus,
  Power,
  RefreshCcw,
  Search,
  Settings,
  SlidersHorizontal,
  SquareTerminal,
  Tag,
  Trash2,
  X,
} from "lucide-react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { skillmgr } from "../wailsjs/go/models";
import { cn } from "./lib/utils";
import { useSkillStore } from "./store/useSkillStore";
import logoUniversal from "./assets/images/logo-universal.png";
import "./App.css";

const statusLabels: Record<string, string> = {
  enabled: "Enabled",
  disabled: "Disabled",
  conflict: "Conflict",
  invalid: "Invalid",
  "missing-source": "Missing Source",
  "missing-path": "Missing Path",
  error: "Error",
};

const statusClass: Record<string, string> = {
  enabled: "status-pill--enabled",
  disabled: "status-pill--disabled",
  conflict: "status-pill--conflict",
  invalid: "status-pill--invalid",
  error: "status-pill--invalid",
  "missing-source": "status-pill--missing",
  "missing-path": "status-pill--missing",
};

const SOURCE_WIDTH_KEY = "skill-manager:source-panel-width";
const DETAIL_WIDTH_KEY = "skill-manager:detail-panel-width";
const DEFAULT_SOURCE_WIDTH = 260;
const DEFAULT_DETAIL_WIDTH = 340;
const MIN_SOURCE_WIDTH = 180;
const MAX_SOURCE_WIDTH = 420;
const MIN_DETAIL_WIDTH = 220;
const MAX_DETAIL_WIDTH = 560;
const RESIZE_HANDLE_WIDTH = 8;
const SKILLS_COLUMNS_KEY = "skill-manager:skills-column-widths";
const skillColumnKeys = ["selection", "enabled", "skill", "tags", "source", "status"] as const;
type SkillColumnKey = (typeof skillColumnKeys)[number];
type SkillColumnWidths = Record<SkillColumnKey, number>;
type RepositoryPanelItem = skillmgr.Repository;
type CloneDraft = { source: skillmgr.Repository; parentDir: string };
type WorkbenchLayout = "desktop" | "split" | "compact";
type CompactView = "skills" | "detail";
type TagPickerState =
  | { mode: "single"; skillId: string; anchor: DOMRect }
  | { mode: "bulk"; anchor: DOMRect };
const DEFAULT_SKILL_COLUMN_WIDTHS: SkillColumnWidths = {
  selection: 7,
  enabled: 10,
  skill: 29,
  tags: 24,
  source: 15,
  status: 15,
};
const MIN_SKILL_COLUMN_WIDTHS: SkillColumnWidths = {
  selection: 6,
  enabled: 8,
  skill: 22,
  tags: 14,
  source: 12,
  status: 12,
};
const TAG_TONES = [
  { backgroundColor: "#e5f6ee", borderColor: "#9ad9bf", color: "#126747" },
  { backgroundColor: "#dff4f5", borderColor: "#8dd2d8", color: "#0d6372" },
  { backgroundColor: "#eeeafb", borderColor: "#c5b7f0", color: "#57439c" },
  { backgroundColor: "#fff1d5", borderColor: "#e5be69", color: "#87500d" },
  { backgroundColor: "#fde8ed", borderColor: "#efacbb", color: "#96304a" },
  { backgroundColor: "#e8f0ff", borderColor: "#adc8ff", color: "#285a9f" },
  { backgroundColor: "#edf4dc", borderColor: "#c3d985", color: "#506b18" },
  { backgroundColor: "#f3ecdf", borderColor: "#d8bd90", color: "#705226" },
] as const;

function App() {
  const {
    inventory,
    selectedSkillId,
    selectedSourceId,
    statusFilter,
    query,
    loading,
    loadingLabel,
    error,
    pullResults,
    setInventory,
    load,
    rescan,
    addSource,
    browseAndAddSource,
    browseForTarget,
    browseForSyncFolder,
    browseForRepositoryFolder,
    removeSource,
    renameSource,
    pullRepository,
    enableSkill,
    enableSkills,
    disableSkill,
    disableSkills,
    cloneRepository,
    useExistingRepository,
    resolveConflict,
    saveConfig,
    saveLLMConfig,
    generateSkillProfile,
    readSkillEnv,
    saveSkillEnv,
    saveSkillTags,
    addSkillTags,
    listSkillFiles,
    readSkillFilePreview,
    openInTerminal,
    openInVSCode,
    openPath,
    selectSkill,
    setSelectedSourceId,
    setStatusFilter,
    setQuery,
    clearError,
  } = useSkillStore();
  const [addSourceOpen, setAddSourceOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [sourceToEdit, setSourceToEdit] = useState<RepositoryPanelItem>();
  const [sourceToRemove, setSourceToRemove] = useState<RepositoryPanelItem>();
  const [cloneDraft, setCloneDraft] = useState<CloneDraft>();
  const [sourcePath, setSourcePath] = useState("");
  const [sourcePanelWidth, setSourcePanelWidth] = useState(() =>
    readStoredWidth(SOURCE_WIDTH_KEY, DEFAULT_SOURCE_WIDTH, MIN_SOURCE_WIDTH, MAX_SOURCE_WIDTH),
  );
  const [detailPanelWidth, setDetailPanelWidth] = useState(() =>
    readStoredWidth(DETAIL_WIDTH_KEY, DEFAULT_DETAIL_WIDTH, MIN_DETAIL_WIDTH, MAX_DETAIL_WIDTH),
  );
  const [skillColumnWidths, setSkillColumnWidths] = useState(readStoredSkillColumnWidths);
  const [generatingProfileIds, setGeneratingProfileIds] = useState<Set<string>>(() => new Set());
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  const [selectedSkillIds, setSelectedSkillIds] = useState<Set<string>>(() => new Set());
  const [tagPicker, setTagPicker] = useState<TagPickerState>();
  const workbenchLayout = useWorkbenchLayout();
  const [repositoryDrawerOpen, setRepositoryDrawerOpen] = useState(false);
  const [compactView, setCompactView] = useState<CompactView>("skills");
  const skillsTableRef = useRef<HTMLDivElement>(null);
  const selectAllRef = useRef<HTMLInputElement>(null);
  const requestedProfilesRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    load();
    const unsubscribe = EventsOn("inventory:changed", (next: skillmgr.Inventory) => {
      setInventory(next);
    });
    return unsubscribe;
  }, [load, setInventory]);

  useEffect(() => {
    localStorage.setItem(SOURCE_WIDTH_KEY, String(sourcePanelWidth));
  }, [sourcePanelWidth]);

  useEffect(() => {
    localStorage.setItem(DETAIL_WIDTH_KEY, String(detailPanelWidth));
  }, [detailPanelWidth]);

  useEffect(() => {
    localStorage.setItem(SKILLS_COLUMNS_KEY, JSON.stringify(skillColumnWidths));
  }, [skillColumnWidths]);

  const allTags = useMemo(() => {
    const tags = new Set<string>();
    for (const skill of inventory?.skills ?? []) {
      for (const tag of skill.tags ?? []) {
        const normalizedTag = tag.trim();
        if (normalizedTag) {
          tags.add(normalizedTag);
        }
      }
    }
    return Array.from(tags).sort((left, right) => left.localeCompare(right));
  }, [inventory?.skills]);

  useEffect(() => {
    if (selectedTags.length === 0) return;
    const availableTags = new Set(allTags);
    setSelectedTags((current) => current.filter((tag) => availableTags.has(tag)));
  }, [allTags, selectedTags.length]);

  const selectedTagSet = useMemo(() => new Set(selectedTags), [selectedTags]);

  const filteredSkills = useMemo(() => {
    const skills = inventory?.skills ?? [];
    const normalizedQuery = query.trim().toLowerCase();
    return skills.filter((skill) => {
      const matchesSource =
        selectedSourceId === "all" || skill.sourceKey === selectedSourceId || skill.sourceId === selectedSourceId || skill.repoId === selectedSourceId;
      const matchesStatus = statusFilter === "all" || skill.status === statusFilter;
      const matchesTags = selectedTagSet.size === 0 || (skill.tags ?? []).some((tag) => selectedTagSet.has(tag));
      const matchesQuery =
        normalizedQuery.length === 0 ||
        skill.name.toLowerCase().includes(normalizedQuery) ||
        (skill.displayName ?? "").toLowerCase().includes(normalizedQuery) ||
        (skill.repoSubpath ?? "").toLowerCase().includes(normalizedQuery) ||
        skill.sourcePath.toLowerCase().includes(normalizedQuery) ||
        (skill.tags ?? []).some((tag) => tag.toLowerCase().includes(normalizedQuery));
      return matchesSource && matchesStatus && matchesQuery && matchesTags;
    });
  }, [inventory?.skills, query, selectedSourceId, selectedTagSet, statusFilter]);

  const selectedSkill =
    filteredSkills.find((skill) => skill.id === selectedSkillId) ??
    inventory?.skills?.find((skill) => skill.id === selectedSkillId) ??
    filteredSkills[0];
  const selectedSkillIdList = Array.from(selectedSkillIds);
  const filteredSelectedCount = filteredSkills.reduce(
    (count, skill) => count + (selectedSkillIds.has(skill.id) ? 1 : 0),
    0,
  );
  const allFilteredSelected = filteredSkills.length > 0 && filteredSelectedCount === filteredSkills.length;
  const workbenchGridColumns = buildWorkbenchGridColumns(workbenchLayout, sourcePanelWidth, detailPanelWidth);

  useEffect(() => {
    const availableSkillIds = new Set((inventory?.skills ?? []).map((skill) => skill.id));
    setSelectedSkillIds((current) => {
      const next = new Set(Array.from(current).filter((skillId) => availableSkillIds.has(skillId)));
      return next.size === current.size ? current : next;
    });
  }, [inventory?.skills]);

  useEffect(() => {
    if (selectAllRef.current) {
      selectAllRef.current.indeterminate = filteredSelectedCount > 0 && !allFilteredSelected;
    }
  }, [allFilteredSelected, filteredSelectedCount]);

  useEffect(() => {
    if (workbenchLayout === "desktop") {
      setRepositoryDrawerOpen(false);
    }
  }, [workbenchLayout]);

  useEffect(() => {
    if (!repositoryDrawerOpen) return;
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") setRepositoryDrawerOpen(false);
    }
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [repositoryDrawerOpen]);

  function maybeGenerateProfileForSkill(skill: skillmgr.Skill) {
    if (
      !inventory?.syncConfigured ||
      !llmConfigReady(inventory.llmConfig) ||
      !skill.syncId ||
      !skill.sourcePath ||
      profileComplete(skill.profile)
    ) {
      return;
    }
    const requestKey = skill.syncId || skill.id;
    if (requestedProfilesRef.current.has(requestKey)) {
      return;
    }
    requestedProfilesRef.current.add(requestKey);
    void generateProfile(skill.id, false);
  }

  function selectSkillWithProfile(skill: skillmgr.Skill) {
    selectSkill(skill.id);
    maybeGenerateProfileForSkill(skill);
    if (workbenchLayout === "compact") {
      setCompactView("detail");
    }
  }

  async function generateProfile(skillId: string, force = false) {
    setGeneratingProfileIds((current) => {
      const next = new Set(current);
      next.add(skillId);
      return next;
    });
    try {
      await generateSkillProfile(skillId, force);
    } finally {
      setGeneratingProfileIds((current) => {
        const next = new Set(current);
        next.delete(skillId);
        return next;
      });
    }
  }

  async function submitSource() {
    if (!sourcePath.trim()) return;
    await addSource(sourcePath.trim());
    setSourcePath("");
    setAddSourceOpen(false);
  }

  function toggleTagFilter(tag: string) {
    setSelectedSourceId("all");
    setSelectedTags((current) =>
      current.includes(tag)
        ? current.filter((item) => item !== tag)
        : [...current, tag].sort((left, right) => left.localeCompare(right)),
    );
  }

  function toggleSkillSelection(skillId: string) {
    setSelectedSkillIds((current) => {
      const next = new Set(current);
      if (next.has(skillId)) {
        next.delete(skillId);
      } else {
        next.add(skillId);
      }
      return next;
    });
  }

  function toggleFilteredSelection() {
    setSelectedSkillIds((current) => {
      const next = new Set(current);
      if (allFilteredSelected) {
        for (const skill of filteredSkills) next.delete(skill.id);
      } else {
        for (const skill of filteredSkills) next.add(skill.id);
      }
      return next;
    });
  }

  function openSingleTagPicker(skillId: string, anchor: HTMLElement) {
    setTagPicker({ mode: "single", skillId, anchor: anchor.getBoundingClientRect() });
  }

  function openBulkTagPicker(anchor: HTMLElement) {
    setTagPicker({ mode: "bulk", anchor: anchor.getBoundingClientRect() });
  }

  async function addSingleSkillTag(skillId: string, tag: string) {
    const skill = inventory?.skills?.find((item) => item.id === skillId);
    if (!skill) return;
    const nextTags = cleanUiTags([...(skill.tags ?? []), tag]);
    setTagPicker(undefined);
    await saveSkillTags(skillId, nextTags);
  }

  async function addTagsToSelectedSkills(tags: string[]) {
    if (selectedSkillIdList.length === 0 || tags.length === 0) return;
    setTagPicker(undefined);
    await addSkillTags(selectedSkillIdList, tags);
  }

  function startColumnResize(kind: "source" | "detail", event: React.PointerEvent) {
    event.preventDefault();
    const startX = event.clientX;
    const startSourceWidth = sourcePanelWidth;
    const startDetailWidth = detailPanelWidth;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const delta = moveEvent.clientX - startX;
      if (kind === "source") {
        setSourcePanelWidth(clamp(startSourceWidth + delta, MIN_SOURCE_WIDTH, MAX_SOURCE_WIDTH));
      } else {
        setDetailPanelWidth(clamp(startDetailWidth - delta, MIN_DETAIL_WIDTH, MAX_DETAIL_WIDTH));
      }
    };

    const stopResize = () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", stopResize);
      window.removeEventListener("pointercancel", stopResize);
    };

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", stopResize);
    window.addEventListener("pointercancel", stopResize);
  }

  function startSkillColumnResize(leftKey: SkillColumnKey, rightKey: SkillColumnKey, event: React.PointerEvent) {
    event.preventDefault();
    event.stopPropagation();
    const tableWidth = skillsTableRef.current?.getBoundingClientRect().width ?? 0;
    if (tableWidth <= 0) return;
    const startX = event.clientX;
    const startWidths = { ...skillColumnWidths };
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const handlePointerMove = (moveEvent: PointerEvent) => {
      const rawDelta = ((moveEvent.clientX - startX) / tableWidth) * 100;
      const minDelta = MIN_SKILL_COLUMN_WIDTHS[leftKey] - startWidths[leftKey];
      const maxDelta = startWidths[rightKey] - MIN_SKILL_COLUMN_WIDTHS[rightKey];
      const delta = clamp(rawDelta, minDelta, maxDelta);
      setSkillColumnWidths({
        ...startWidths,
        [leftKey]: startWidths[leftKey] + delta,
        [rightKey]: startWidths[rightKey] - delta,
      });
    };

    const stopResize = () => {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("pointermove", handlePointerMove);
      window.removeEventListener("pointerup", stopResize);
      window.removeEventListener("pointercancel", stopResize);
    };

    window.addEventListener("pointermove", handlePointerMove);
    window.addEventListener("pointerup", stopResize);
    window.addEventListener("pointercancel", stopResize);
  }

  function resizePanelByKeyboard(kind: "source" | "detail", event: React.KeyboardEvent) {
    const delta = keyboardResizeDelta(event);
    if (!delta) return;
    event.preventDefault();
    if (kind === "source") {
      setSourcePanelWidth((width) => clamp(width + delta, MIN_SOURCE_WIDTH, MAX_SOURCE_WIDTH));
    } else {
      setDetailPanelWidth((width) => clamp(width - delta, MIN_DETAIL_WIDTH, MAX_DETAIL_WIDTH));
    }
  }

  function resizeSkillColumnByKeyboard(leftKey: SkillColumnKey, rightKey: SkillColumnKey, event: React.KeyboardEvent) {
    const delta = keyboardResizeDelta(event);
    if (!delta) return;
    event.preventDefault();
    setSkillColumnWidths((widths) => adjustSkillColumnWidths(widths, leftKey, rightKey, delta));
  }

  async function prepareMissingRepositoryClone(source: skillmgr.Repository) {
    const parentDir = await browseForRepositoryFolder();
    if (!parentDir) return;
    setCloneDraft({ source, parentDir });
  }

  function selectRepository(sourceId: string) {
    setSelectedSourceId(sourceId);
    setRepositoryDrawerOpen(false);
    if (workbenchLayout === "compact") {
      setCompactView("skills");
    }
  }

  if (inventory && !inventory.syncConfigured) {
    return (
      <SyncSetupScreen
        error={error}
        loading={loading}
        loadingLabel={loadingLabel}
        onClearError={clearError}
        onChooseFolder={async () => {
          const folder = await browseForSyncFolder();
          if (!folder) return;
          const config = skillmgr.Config.createFrom(inventory.config);
          config.sync = skillmgr.SyncConfig.createFrom({ folder });
          await saveConfig(config);
        }}
      />
    );
  }

  return (
    <div className="app-shell flex h-screen min-w-0 flex-col overflow-hidden bg-background">
      <header className="app-topbar flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-5">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <div className="brand-lockup">
            <img className="brand-mark" src={logoUniversal} alt="" width={36} height={36} />
            <div className="min-w-0">
              <h1 className="brand-title">Skill Manager</h1>
              <p className="brand-subtitle max-w-[calc(100vw-2rem)] truncate sm:max-w-[520px]">
                Linked to {targetDirsLabel(inventory?.config?.targetDirs)}
              </p>
            </div>
          </div>
          <div className="sync-route" aria-hidden="true">
            <span />
            <span />
            <span />
          </div>
          <div className="hidden min-w-0 text-xs text-muted-foreground lg:block">
            <p className="route-caption">
              Sources scan into a managed target. Conflicts stay visible until you choose the active skill.
            </p>
          </div>
          {inventory && <SummaryBar summary={inventory.summary} />}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="outline"
            onClick={rescan}
            disabled={loading}
            title="Scan every repository again and reconcile the shared catalog with this machine."
          >
            <RefreshCcw aria-hidden="true" className={cn("h-4 w-4", loading && "animate-spin")} />
            <span className="topbar-command-label">Rescan All</span>
          </Button>
          <IconButton title="Open target folder, sync, scan, and LLM settings." onClick={() => setSettingsOpen(true)}>
            <Settings aria-hidden="true" className="h-4 w-4" />
          </IconButton>
        </div>
      </header>

      {error && (
        <div className="flex items-center justify-between border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-700">
          <span>{error}</span>
          <button
            className="rounded p-1 hover:bg-rose-100"
            onClick={clearError}
            title="Hide this error message."
            aria-label="Dismiss error"
          >
            <X aria-hidden="true" className="h-4 w-4" />
          </button>
        </div>
      )}

      {loading && <LoadingOverlay label={loadingLabel || "Working..."} />}

      {workbenchLayout === "compact" && (
        <CompactWorkbenchTabs
          activeView={compactView}
          detailAvailable={Boolean(selectedSkill)}
          onOpenRepositories={() => setRepositoryDrawerOpen(true)}
          onSelectView={setCompactView}
        />
      )}

      <main
        className="workbench-grid grid min-h-0 flex-1 overflow-hidden"
        data-layout={workbenchLayout}
        style={{
          gridTemplateColumns: workbenchGridColumns,
        }}
      >
        {workbenchLayout === "desktop" && (
          <>
            <RepositoryPanel
              loading={loading}
              pullResults={pullResults}
              repositories={inventory?.repositories ?? []}
              selectedSourceId={selectedSourceId}
              onAdd={() => setAddSourceOpen(true)}
              onClone={prepareMissingRepositoryClone}
              onOpenPath={openPath}
              onPull={(source) => pullRepository(source.repoId)}
              onRemove={setSourceToRemove}
              onRename={setSourceToEdit}
              onSelect={selectRepository}
              onUseExisting={(source) => useExistingRepository(source.repoId)}
            />
            <ResizeHandle
              label="Resize Repositories"
              onKeyDown={(event) => resizePanelByKeyboard("source", event)}
              onPointerDown={(event) => startColumnResize("source", event)}
            />
          </>
        )}

        {(workbenchLayout !== "compact" || compactView === "skills") && (
        <section
          id="workbench-skills-panel"
          className="workbench-panel workbench-panel--skills flex min-h-0 min-w-0 flex-col overflow-hidden bg-slate-50"
          role={workbenchLayout === "compact" ? "tabpanel" : undefined}
          aria-label={workbenchLayout === "compact" ? "Skills" : undefined}
        >
          <PanelHeader title={selectedSkillIds.size > 0 ? `${selectedSkillIds.size} selected` : "Skills"}>
            <div className="flex items-center gap-2">
              {workbenchLayout === "split" && (
                <IconButton
                  onClick={() => setRepositoryDrawerOpen(true)}
                  title="Open the repository drawer to filter skills or manage repository checkouts."
                >
                  <PanelLeftOpen aria-hidden="true" className="h-4 w-4" />
                </IconButton>
              )}
              {selectedSkillIds.size > 0 ? (
                <>
                  <IconButton
                    onClick={() => void enableSkills(selectedSkillIdList)}
                    disabled={loading}
                    className="icon-button--primary"
                    title="Enable the selected skills that are available and valid; unavailable or invalid selections are skipped."
                  >
                    <Check aria-hidden="true" className="h-4 w-4" />
                  </IconButton>
                  <IconButton
                    onClick={() => void disableSkills(selectedSkillIdList)}
                    disabled={loading}
                    title="Disable active skills in the selection by removing their managed target links."
                  >
                    <Power aria-hidden="true" className="h-4 w-4" />
                  </IconButton>
                  <IconButton
                    onClick={(event) => openBulkTagPicker(event.currentTarget)}
                    disabled={loading}
                    title="Choose one or more tags to append to every selected skill without replacing existing tags."
                  >
                    <Tag aria-hidden="true" className="h-4 w-4" />
                  </IconButton>
                  <IconButton
                    onClick={() => setSelectedSkillIds(new Set())}
                    title="Clear the current skill selection without changing any skills."
                  >
                    <X aria-hidden="true" className="h-4 w-4" />
                  </IconButton>
                </>
              ) : (
                <IconButton
                  onClick={rescan}
                  disabled={loading}
                  title="Scan repositories again and reconcile the shared skill state on this machine."
                >
                  <RefreshCcw aria-hidden="true" className={cn("h-4 w-4", loading && "animate-spin")} />
                </IconButton>
              )}
            </div>
          </PanelHeader>
          {pullResults.bulk && (
            <div className="border-b border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-700">{pullResults.bulk}</div>
          )}
          <div className="filter-bar shrink-0 border-b border-border bg-white p-3">
            <div className="flex flex-wrap gap-2">
              <div className="relative min-w-0 flex-1">
                <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                <input
                  aria-label="Search skills"
                  autoComplete="off"
                  name="skill-search"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search Skill…"
                  className="h-9 w-full rounded-md border border-input bg-white pl-8 pr-3 text-sm"
                />
              </div>
              <select
                aria-label="Filter by source"
                name="source-filter"
                value={selectedSourceId}
                onChange={(event) => setSelectedSourceId(event.target.value)}
                className="h-9 min-w-[150px] flex-1 rounded-md border border-input bg-white px-2 text-sm sm:flex-none"
              >
                <option value="all">Any source</option>
                {(inventory?.repositories ?? []).map((source) => (
                  <option key={repositoryItemId(source)} value={repositoryItemId(source)}>
                    {repositoryItemTitle(source)}
                  </option>
                ))}
                {(inventory?.sources ?? []).map((source) => (
                  <option key={source.id} value={source.id}>
                    {source.alias || basename(source.path)}
                  </option>
                ))}
              </select>
              <select
                aria-label="Filter by status"
                name="status-filter"
                value={statusFilter}
                onChange={(event) => setStatusFilter(event.target.value)}
                className="h-9 min-w-[130px] flex-1 rounded-md border border-input bg-white px-2 text-sm sm:flex-none"
              >
                <option value="all">Any status</option>
                {Object.entries(statusLabels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </div>
            {allTags.length > 0 && (
              <div className="tag-filter-strip mt-3 flex min-w-0 flex-wrap gap-1.5">
                {allTags.map((tag) => (
                  <TagFilterButton
                    key={tag}
                    tag={tag}
                    selected={selectedTagSet.has(tag)}
                    onClick={() => toggleTagFilter(tag)}
                  />
                ))}
              </div>
            )}
          </div>
          <div ref={skillsTableRef} className="min-h-0 flex-1 overflow-auto">
            <table className="w-full table-fixed border-collapse text-sm">
              <colgroup>
                {skillColumnKeys.map((key) => (
                  <col key={key} style={{ width: `${skillColumnWidths[key]}%` }} />
                ))}
              </colgroup>
              <thead className="text-left text-xs font-medium text-muted-foreground">
                <tr className="border-b border-border">
                  <SelectionHeaderCell
                    checked={allFilteredSelected}
                    disabled={filteredSkills.length === 0}
                    inputRef={selectAllRef}
                    onChange={toggleFilteredSelection}
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("selection", "enabled", event)}
                    onResize={(event) => startSkillColumnResize("selection", "enabled", event)}
                  />
                  <SkillHeaderCell
                    label="On"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("enabled", "skill", event)}
                    onResize={(event) => startSkillColumnResize("enabled", "skill", event)}
                  />
                  <SkillHeaderCell
                    label="Skill"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("skill", "tags", event)}
                    onResize={(event) => startSkillColumnResize("skill", "tags", event)}
                  />
                  <SkillHeaderCell
                    label="Tags"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("tags", "source", event)}
                    onResize={(event) => startSkillColumnResize("tags", "source", event)}
                  />
                  <SkillHeaderCell
                    label="Source"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("source", "status", event)}
                    onResize={(event) => startSkillColumnResize("source", "status", event)}
                  />
                  <SkillHeaderCell label="Status" />
                </tr>
              </thead>
              <tbody>
                {filteredSkills.map((skill) => (
                  <tr
                    key={skill.id}
                    aria-selected={selectedSkill?.id === skill.id}
                    className={cn(
                      "skill-row cursor-pointer border-b border-border bg-white hover:bg-blue-50/50",
                      selectedSkill?.id === skill.id && "skill-row--selected bg-blue-50",
                    )}
                    onClick={() => selectSkillWithProfile(skill)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        selectSkillWithProfile(skill);
                      }
                    }}
                    role="button"
                    tabIndex={0}
                    title={`Open the detail panel for ${skill.displayName || skill.name}.`}
                  >
                    <td className="overflow-hidden px-3 py-2">
                      <input
                        aria-label={`Select ${skill.displayName || skill.name} for bulk actions`}
                        checked={selectedSkillIds.has(skill.id)}
                        className="skill-select-checkbox h-4 w-4 accent-[var(--sm-link)]"
                        onChange={() => toggleSkillSelection(skill.id)}
                        onClick={(event) => event.stopPropagation()}
                        onKeyDown={(event) => event.stopPropagation()}
                        title="Include this skill in bulk Enable, Disable, and Add Tags actions."
                        type="checkbox"
                      />
                    </td>
                    <td className="overflow-hidden px-2 py-2">
                      <SkillSwitch
                        skill={skill}
                        onEnable={() => (skill.status === "conflict" ? resolveConflict(skill.id) : enableSkill(skill.id))}
                        onDisable={() => disableSkill(skill.id)}
                      />
                    </td>
                    <td className="min-w-0 overflow-hidden px-3 py-2">
                      <div className="truncate font-medium">{skill.displayName || skill.name}</div>
                      <div className="truncate text-xs text-muted-foreground">
                        {skill.description || skill.repoSubpath || "No summary yet"}
                      </div>
                    </td>
                    <td className="min-w-0 px-3 py-2">
                      <SkillTagsCell
                        skill={skill}
                        onOpenPicker={(anchor) => openSingleTagPicker(skill.id, anchor)}
                      />
                    </td>
                    <td className="min-w-0 overflow-hidden px-3 py-2 text-muted-foreground">
                      <div className="truncate">{skill.sourceAlias || skill.repoId || skill.sourceId}</div>
                      {skill.repoSubpath && <div className="truncate text-xs">{skill.repoSubpath}</div>}
                    </td>
                    <td className="min-w-0 overflow-hidden px-3 py-2">
                      <StatusPill status={skill.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {filteredSkills.length === 0 && (
              <div className="empty-state p-8 text-center text-sm text-muted-foreground">
                No skills match these filters. Clear search, tags, or choose another status.
              </div>
            )}
          </div>
        </section>
        )}

        {workbenchLayout !== "compact" && (
          <ResizeHandle
            label="Resize Skill Detail"
            onKeyDown={(event) => resizePanelByKeyboard("detail", event)}
            onPointerDown={(event) => startColumnResize("detail", event)}
          />
        )}

        {(workbenchLayout !== "compact" || compactView === "detail") && (
        <aside
          id="workbench-detail-panel"
          className="workbench-panel workbench-panel--detail flex min-h-0 min-w-0 flex-col overflow-hidden bg-white"
          role={workbenchLayout === "compact" ? "tabpanel" : undefined}
          aria-label={workbenchLayout === "compact" ? "Skill Detail" : undefined}
        >
          <PanelHeader title="Skill Detail">
            <div className="flex items-center gap-2">
              {selectedSkill &&
                selectedSkill.sourcePath &&
                selectedSkill.status !== "missing-source" &&
                selectedSkill.status !== "missing-path" && (
                  <IconButton
                    title="Open this skill's original source folder in Finder."
                    onClick={() => openPath(selectedSkill.sourcePath)}
                  >
                    <Folder aria-hidden="true" className="h-4 w-4" />
                  </IconButton>
                )}
              <IconButton
                title="Open the selected skill's source folder in VS Code."
                disabled={!selectedSkill}
                onClick={() => selectedSkill && openInVSCode(selectedSkill.sourcePath)}
              >
                <VSCodeIcon className="h-4 w-4" />
              </IconButton>
              {selectedSkill &&
                selectedSkill.sourcePath &&
                selectedSkill.status !== "missing-source" &&
                selectedSkill.status !== "missing-path" && (
                  <IconButton
                    title="Open Terminal.app with this skill's original source directory as the working directory."
                    onClick={() => openInTerminal(selectedSkill.sourcePath)}
                  >
                    <SquareTerminal aria-hidden="true" className="h-4 w-4" />
                  </IconButton>
                )}
            </div>
          </PanelHeader>
          <SkillDetail
            skill={selectedSkill}
            syncConfigured={Boolean(inventory?.syncConfigured)}
            llmConfig={inventory?.llmConfig}
            isGeneratingProfile={Boolean(selectedSkill && generatingProfileIds.has(selectedSkill.id))}
            onResolve={resolveConflict}
            onGenerateProfile={generateProfile}
            onReadEnv={readSkillEnv}
            onSaveEnv={saveSkillEnv}
            onSaveTags={saveSkillTags}
            onListFiles={listSkillFiles}
            onReadFilePreview={readSkillFilePreview}
          />
        </aside>
        )}
      </main>

      {workbenchLayout !== "desktop" && repositoryDrawerOpen && (
        <div
          className="repository-drawer-backdrop fixed inset-0 z-[60] flex bg-slate-950/35"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setRepositoryDrawerOpen(false);
          }}
          role="presentation"
        >
          <div
            aria-label="Repositories"
            aria-modal="true"
            className="repository-drawer h-full min-h-0 overflow-hidden bg-white shadow-2xl"
            role="dialog"
          >
            <RepositoryPanel
              loading={loading}
              pullResults={pullResults}
              repositories={inventory?.repositories ?? []}
              selectedSourceId={selectedSourceId}
              onAdd={() => {
                setRepositoryDrawerOpen(false);
                setAddSourceOpen(true);
              }}
              onClone={async (source) => {
                setRepositoryDrawerOpen(false);
                await prepareMissingRepositoryClone(source);
              }}
              onClose={() => setRepositoryDrawerOpen(false)}
              onOpenPath={openPath}
              onPull={(source) => pullRepository(source.repoId)}
              onRemove={(source) => {
                setRepositoryDrawerOpen(false);
                setSourceToRemove(source);
              }}
              onRename={(source) => {
                setRepositoryDrawerOpen(false);
                setSourceToEdit(source);
              }}
              onSelect={selectRepository}
              onUseExisting={async (source) => {
                setRepositoryDrawerOpen(false);
                await useExistingRepository(source.repoId);
              }}
            />
          </div>
        </div>
      )}

      {tagPicker && (
        <TagPickerPopover
          allTags={allTags}
          anchor={tagPicker.anchor}
          existingTags={
            tagPicker.mode === "single"
              ? inventory?.skills?.find((skill) => skill.id === tagPicker.skillId)?.tags ?? []
              : []
          }
          mode={tagPicker.mode}
          selectedCount={selectedSkillIds.size}
          onClose={() => setTagPicker(undefined)}
          onSingleSelect={(tag) =>
            tagPicker.mode === "single" ? addSingleSkillTag(tagPicker.skillId, tag) : Promise.resolve()
          }
          onBulkSubmit={addTagsToSelectedSkills}
        />
      )}

      {addSourceOpen && (
        <Modal title="Add Repository" onClose={() => setAddSourceOpen(false)}>
          <div className="space-y-4">
            <label className="block text-sm font-medium">
              Git Repository
              <div className="mt-2 flex gap-2">
                <input
                    autoComplete="off"
                    name="source-directory"
                  value={sourcePath}
                  onChange={(event) => setSourcePath(event.target.value)}
                  className="h-9 min-w-0 flex-1 rounded-md border border-input px-3 text-sm"
                  placeholder="/Users/yusuf/dev/my-skills…"
                />
                <Button
                  variant="outline"
                  title="Open a folder picker and add the selected Git repository to the shared catalog."
                  onClick={async () => {
                    await browseAndAddSource();
                    setAddSourceOpen(false);
                  }}
                >
                  Browse
                </Button>
              </div>
            </label>
            <div className="rounded-md border border-border bg-slate-50 p-3 text-xs text-muted-foreground">
              The selected folder must be a Git repository with a usable remote. It is scanned recursively for SKILL.md.
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setAddSourceOpen(false)} title="Close this dialog without adding a source.">
                Cancel
              </Button>
              <Button title="Validate the typed Git repository, scan its skills, and add them to the shared catalog." onClick={submitSource}>
                Add
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {cloneDraft && (
        <CloneDestinationModal
          source={cloneDraft.source}
          parentDir={cloneDraft.parentDir}
          onClose={() => setCloneDraft(undefined)}
          onClone={(folderName) =>
            cloneRepository(
              cloneDraft.source.repoId,
              cloneDraft.source.cloneUrl ?? "",
              cloneDraft.parentDir,
              folderName,
            )
          }
        />
      )}

      {settingsOpen && inventory && (
        <SettingsModal
          inventory={inventory}
          onClose={() => setSettingsOpen(false)}
          onBrowseTarget={browseForTarget}
          onBrowseSyncFolder={browseForSyncFolder}
          onSave={async (config, llmConfig) => {
            await saveConfig(config);
            if (config.sync?.folder) {
              await saveLLMConfig(llmConfig);
            }
            setSettingsOpen(false);
          }}
        />
      )}

      {sourceToEdit && (
        <SourceAliasModal
          source={sourceToEdit}
          onClose={() => setSourceToEdit(undefined)}
          onSave={async (alias) => {
            await renameSource(sourceToEdit.repoId, alias);
            setSourceToEdit(undefined);
          }}
        />
      )}

      {sourceToRemove && (
        <RemoveSourceModal
          source={sourceToRemove}
          onClose={() => setSourceToRemove(undefined)}
          onRemove={async () => {
            await removeSource(sourceToRemove.repoId);
            setSourceToRemove(undefined);
          }}
        />
      )}

    </div>
  );
}

type RepositoryPanelProps = {
  repositories: skillmgr.Repository[];
  selectedSourceId: string;
  loading: boolean;
  pullResults: Record<string, string>;
  onAdd: () => void;
  onSelect: (sourceId: string) => void;
  onOpenPath: (path: string) => Promise<void>;
  onRename: (source: RepositoryPanelItem) => void;
  onUseExisting: (source: RepositoryPanelItem) => Promise<void>;
  onPull: (source: RepositoryPanelItem) => Promise<void>;
  onClone: (source: RepositoryPanelItem) => Promise<void>;
  onRemove: (source: RepositoryPanelItem) => void;
  onClose?: () => void;
};

function RepositoryPanel({
  repositories,
  selectedSourceId,
  loading,
  pullResults,
  onAdd,
  onSelect,
  onOpenPath,
  onRename,
  onUseExisting,
  onPull,
  onClone,
  onRemove,
  onClose,
}: RepositoryPanelProps) {
  return (
    <aside className="workbench-panel workbench-panel--sources flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-white">
      <PanelHeader title="Repositories">
        <div className="flex items-center gap-2">
          <IconButton
            title="Choose a Git repository with a remote and add all of its skills to the shared catalog."
            onClick={onAdd}
          >
            <FolderPlus aria-hidden="true" className="h-4 w-4" />
          </IconButton>
          {onClose && (
            <IconButton
              autoFocus
              title="Close the repository drawer and return to the current skill workspace."
              onClick={onClose}
            >
              <X aria-hidden="true" className="h-4 w-4" />
            </IconButton>
          )}
        </div>
      </PanelHeader>
      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3">
        {repositories.map((source) => (
          <div
            key={repositoryItemId(source)}
            role="button"
            tabIndex={0}
            title={`Filter the skill list to items from ${repositoryItemTitle(source)}.`}
            className={cn(
              "source-card w-full cursor-pointer rounded-md border p-3 text-left transition hover:bg-slate-50",
              selectedSourceId === repositoryItemId(source) && "source-card--selected border-blue-300 bg-blue-50",
            )}
            onClick={() => onSelect(repositoryItemId(source))}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                onSelect(repositoryItemId(source));
              }
            }}
          >
            <div className="flex items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="truncate text-sm font-medium">{repositoryItemTitle(source)}</div>
                <div className="truncate text-xs text-muted-foreground">{source.installed ? source.path : source.repoId}</div>
                {source.currentRef && <div className="truncate text-xs text-muted-foreground">{source.currentRef}</div>}
                {!source.installed && <div className="truncate text-xs font-medium text-amber-700">Missing locally</div>}
              </div>
              {!source.installed || source.errorCount > 0 || source.dirty ? (
                <AlertTriangle aria-hidden="true" className="h-4 w-4 shrink-0 text-amber-600" />
              ) : (
                <Circle aria-hidden="true" className="h-4 w-4 shrink-0 text-emerald-600" />
              )}
            </div>
            <div className="mt-3 flex items-center justify-between text-xs text-muted-foreground">
              <span>{source.skillCount} skills</span>
              <span>{source.installed ? formatDate(source.lastScannedAt) : "Shared"}</span>
            </div>
            <div className="mt-3 flex gap-1">
              {source.installed && (
                <SmallAction title="Open this repository folder in Finder." onClick={(event) => action(event, () => void onOpenPath(source.path))}>
                  <ExternalLink aria-hidden="true" className="h-3.5 w-3.5" />
                </SmallAction>
              )}
              {source.installed && (
                <SmallAction title="Rename how this repository appears in Skill Manager." onClick={(event) => action(event, () => onRename(source))}>
                  <SlidersHorizontal aria-hidden="true" className="h-3.5 w-3.5" />
                </SmallAction>
              )}
              {!source.installed && (
                <SmallAction
                  title="Choose an existing local checkout and verify that its Git remote matches this missing repository."
                  onClick={(event) => action(event, () => void onUseExisting(source))}
                >
                  <FolderOpen aria-hidden="true" className="h-3.5 w-3.5" />
                </SmallAction>
              )}
              <SmallAction
                title={
                  !source.installed
                    ? source.cloneUrl
                      ? "Choose a parent folder, name the destination, then shallow-clone this repository and restore its shared skills."
                      : "Clone is unavailable because the shared source does not contain a clone URL."
                    : source.dirty
                      ? "Pull is disabled until local repository changes are committed or stashed."
                      : "Fetch the current upstream branch and fast-forward only its changed paths; shallow checkouts remain shallow."
                }
                disabled={loading || (source.installed ? source.dirty : !source.cloneUrl)}
                onClick={(event) => action(event, () => void (source.installed ? onPull(source) : onClone(source)))}
              >
                <CloudDownload aria-hidden="true" className={cn("h-3.5 w-3.5", loading && "animate-pulse")} />
              </SmallAction>
              {source.installed && (
                <SmallAction title="Remove this repository from Skill Manager without deleting its files." onClick={(event) => action(event, () => onRemove(source))}>
                  <Trash2 aria-hidden="true" className="h-3.5 w-3.5" />
                </SmallAction>
              )}
            </div>
            {pullResults[source.repoId] && (
              <div className="mt-2 truncate rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs text-emerald-700">
                {pullResults[source.repoId]}
              </div>
            )}
          </div>
        ))}
        <button
          aria-label="Show skills from all sources"
          title="Clear the source filter and show skills from every repository."
          className={cn(
            "source-card source-card--all w-full rounded-md border p-3 text-left text-sm",
            selectedSourceId === "all" && "source-card--selected border-blue-300 bg-blue-50",
          )}
          onClick={() => onSelect("all")}
        >
          All Sources
        </button>
      </div>
    </aside>
  );
}

function CompactWorkbenchTabs({
  activeView,
  detailAvailable,
  onOpenRepositories,
  onSelectView,
}: {
  activeView: CompactView;
  detailAvailable: boolean;
  onOpenRepositories: () => void;
  onSelectView: (view: CompactView) => void;
}) {
  function handleArrowKey(event: React.KeyboardEvent, view: CompactView) {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    if (view === "skills" && detailAvailable) onSelectView("detail");
    if (view === "detail") onSelectView("skills");
  }

  return (
    <div className="compact-workbench-tabs flex h-12 shrink-0 items-center gap-2 border-b border-border bg-white px-3" role="tablist" aria-label="Workspace views">
      <IconButton
        onClick={onOpenRepositories}
        title="Open the repository drawer to filter skills or manage repository checkouts."
      >
        <PanelLeftOpen aria-hidden="true" className="h-4 w-4" />
      </IconButton>
      <button
        aria-controls="workbench-skills-panel"
        aria-selected={activeView === "skills"}
        className={cn("compact-workbench-tab", activeView === "skills" && "compact-workbench-tab--active")}
        onClick={() => onSelectView("skills")}
        onKeyDown={(event) => handleArrowKey(event, "skills")}
        role="tab"
        title="Show the searchable skill table and bulk actions."
        type="button"
      >
        <List aria-hidden="true" className="h-4 w-4" />
        Skills
      </button>
      <button
        aria-controls="workbench-detail-panel"
        aria-selected={activeView === "detail"}
        className={cn("compact-workbench-tab", activeView === "detail" && "compact-workbench-tab--active")}
        disabled={!detailAvailable}
        onClick={() => onSelectView("detail")}
        onKeyDown={(event) => handleArrowKey(event, "detail")}
        role="tab"
        title="Show files, tags, status, and metadata for the selected skill."
        type="button"
      >
        <FileText aria-hidden="true" className="h-4 w-4" />
        Detail
      </button>
    </div>
  );
}

function SummaryBar({ summary }: { summary: skillmgr.Summary }) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs">
      <SummaryItem label="Found" value={summary.skillsFound} />
      <SummaryItem label="Enabled" value={summary.enabled} tone="emerald" />
      <SummaryItem label="Conflicts" value={summary.conflicts} tone="amber" />
      <SummaryItem label="Invalid" value={summary.invalid} tone="rose" />
    </div>
  );
}

function SyncSetupScreen({
  error,
  loading,
  loadingLabel,
  onClearError,
  onChooseFolder,
}: {
  error?: string;
  loading: boolean;
  loadingLabel?: string;
  onClearError: () => void;
  onChooseFolder: () => Promise<void>;
}) {
  return (
    <div className="app-shell flex h-screen min-w-0 flex-col overflow-hidden bg-background">
      <header className="app-topbar flex min-h-16 shrink-0 items-center px-5">
        <div className="brand-lockup">
          <img className="brand-mark" src={logoUniversal} alt="" width={36} height={36} />
          <h1 className="brand-title">Skill Manager</h1>
        </div>
      </header>
      {error && (
        <div className="flex items-center justify-between border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-700">
          <span>{error}</span>
          <button className="rounded p-1 hover:bg-rose-100" onClick={onClearError} title="Hide this setup error." aria-label="Dismiss error">
            <X aria-hidden="true" className="h-4 w-4" />
          </button>
        </div>
      )}
      {loading && <LoadingOverlay label={loadingLabel || "Setting up shared storage..."} />}
      <main className="flex min-h-0 flex-1 items-center justify-center bg-slate-50 p-6">
        <section className="w-full max-w-xl border border-border bg-white p-6 shadow-sm">
          <div className="flex items-start gap-4">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-cyan-50 text-cyan-700">
              <CloudDownload aria-hidden="true" className="h-5 w-5" />
            </div>
            <div className="min-w-0">
              <h2 className="text-lg font-semibold">Choose shared storage</h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                Select the iCloud folder that will hold the shared skill catalog.
              </p>
            </div>
          </div>
          <div className="mt-6 flex justify-end">
            <Button
              onClick={() => void onChooseFolder()}
              disabled={loading}
              title="Choose the required iCloud folder, then create or load the shared skill catalog there."
            >
              <FolderPlus aria-hidden="true" className="h-4 w-4" />
              Choose folder
            </Button>
          </div>
        </section>
      </main>
    </div>
  );
}

function LoadingOverlay({ label }: { label: string }) {
  return (
    <div className="loading-overlay fixed inset-0 z-[70] flex items-center justify-center bg-white/72 px-6 backdrop-blur-sm">
      <div className="loading-panel w-full max-w-sm rounded-md border border-border bg-white p-4 shadow-xl">
        <div className="flex items-center gap-3">
          <Loader2 aria-hidden="true" className="h-5 w-5 animate-spin text-[var(--sm-link)]" />
          <div className="min-w-0">
            <div className="truncate text-sm font-medium text-slate-800">{label}</div>
            <div className="mt-1 text-xs text-muted-foreground">This may take a moment for large repositories.</div>
          </div>
        </div>
        <div className="loading-progress mt-4 h-1.5 overflow-hidden rounded-full bg-slate-100">
          <div className="loading-progress-bar h-full rounded-full" />
        </div>
      </div>
    </div>
  );
}

function SummaryItem({ label, value, tone = "slate" }: { label: string; value: number; tone?: string }) {
  const tones: Record<string, string> = {
    slate: "summary-item--slate",
    emerald: "summary-item--emerald",
    amber: "summary-item--amber",
    rose: "summary-item--rose",
  };
  return (
    <span className={cn("summary-item rounded-md px-2 py-1 font-medium", tones[tone])}>
      {value} {label}
    </span>
  );
}

function ResizeHandle({
  label,
  onKeyDown,
  onPointerDown,
}: {
  label: string;
  onKeyDown: (event: React.KeyboardEvent) => void;
  onPointerDown: (event: React.PointerEvent) => void;
}) {
  return (
    <div
      aria-label={label}
      role="separator"
      tabIndex={0}
      title={`${label}: drag this divider or use the left and right arrow keys to change panel width.`}
      onKeyDown={onKeyDown}
      onPointerDown={onPointerDown}
      className="resize-handle group flex min-h-0 cursor-col-resize items-stretch justify-center bg-white"
    >
      <div className="resize-handle-line w-px bg-border transition group-hover:w-1 group-hover:bg-blue-400" />
    </div>
  );
}

function SkillHeaderCell({
  label,
  onKeyResize,
  onResize,
}: {
  label: string;
  onKeyResize?: (event: React.KeyboardEvent) => void;
  onResize?: (event: React.PointerEvent) => void;
}) {
  return (
    <th className="skill-header-cell sticky top-0 z-20 bg-slate-100 px-3 py-2">
      <div className="relative min-w-0">
        <span className="block truncate">{label}</span>
        {onResize && (
          <span
            role="separator"
            aria-label={`Resize ${label} column`}
            tabIndex={0}
            title={`Drag this divider or use the arrow keys to change the ${label} column width.`}
            onKeyDown={onKeyResize}
            onPointerDown={onResize}
            className="column-resize absolute -right-3 top-1/2 h-6 w-2 -translate-y-1/2 cursor-col-resize rounded hover:bg-blue-400/50"
          />
        )}
      </div>
    </th>
  );
}

function SelectionHeaderCell({
  checked,
  disabled,
  inputRef,
  onChange,
  onKeyResize,
  onResize,
}: {
  checked: boolean;
  disabled: boolean;
  inputRef: React.RefObject<HTMLInputElement>;
  onChange: () => void;
  onKeyResize: (event: React.KeyboardEvent) => void;
  onResize: (event: React.PointerEvent) => void;
}) {
  return (
    <th className="skill-header-cell sticky top-0 z-20 bg-slate-100 px-3 py-2">
      <div className="relative flex min-w-0 items-center">
        <input
          ref={inputRef}
          aria-label="Select all skills in the current filtered list"
          checked={checked}
          className="skill-select-checkbox h-4 w-4 accent-[var(--sm-link)]"
          disabled={disabled}
          onChange={onChange}
          title="Select or clear every skill currently shown by the active filters."
          type="checkbox"
        />
        <span
          role="separator"
          aria-label="Resize Selection column"
          tabIndex={0}
          title="Drag this divider or use the arrow keys to change the Selection column width."
          onKeyDown={onKeyResize}
          onPointerDown={onResize}
          className="column-resize absolute -right-3 top-1/2 h-6 w-2 -translate-y-1/2 cursor-col-resize rounded hover:bg-blue-400/50"
        />
      </div>
    </th>
  );
}

function SkillSwitch({
  skill,
  onEnable,
  onDisable,
}: {
  skill: skillmgr.Skill;
  onEnable: () => void;
  onDisable: () => void;
}) {
  const checked = isActiveSkill(skill);
  const disabled = ["invalid", "error", "missing-source", "missing-path"].includes(skill.status);
  const switchTitle = disabled
    ? "This skill cannot be enabled until its source or validation issues are fixed."
    : checked
      ? "Disable this skill and remove its active link from the target folder."
      : skill.status === "conflict"
        ? "Resolve the target conflict by making this skill active."
        : "Enable this skill by linking it into the target folder.";
  return (
    <button
      aria-label={`${checked ? "Disable" : "Enable"} ${skill.name}`}
      aria-checked={checked}
      role="switch"
      title={switchTitle}
      disabled={disabled}
      onClick={(event) => {
        event.stopPropagation();
        checked ? onDisable() : onEnable();
      }}
      className={cn(
        "skill-switch relative h-5 w-9 rounded-full border transition disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "skill-switch--on border-emerald-500 bg-emerald-500" : "skill-switch--off border-slate-300 bg-slate-200",
      )}
    >
      <span
        className={cn(
          "absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition",
          checked ? "left-4" : "left-0.5",
        )}
      />
    </button>
  );
}

function isActiveSkill(skill: skillmgr.Skill) {
  return skill.isActive || skill.status === "enabled";
}

function linkedSkillPaths(skill: skillmgr.Skill) {
  const linkedPaths = (skill.targetStates ?? [])
    .filter((target) => target.isActive && target.symlinkPath)
    .map((target) => target.symlinkPath);

  if (linkedPaths.length > 0) {
    return linkedPaths;
  }

  if (isActiveSkill(skill) && skill.symlinkPath) {
    return [skill.symlinkPath];
  }

  return [];
}

function cleanUiTags(tags: string[]) {
  return Array.from(new Set(tags.map((tag) => tag.trim()).filter(Boolean))).sort((left, right) =>
    left.localeCompare(right),
  );
}

function llmConfigReady(config?: skillmgr.SyncLLMConfig) {
  return Boolean(config?.baseUrl?.trim() && config?.apiKey?.trim() && config?.model?.trim());
}

function profileComplete(profile?: skillmgr.SkillProfile) {
  return Boolean(profile?.summaryZh?.trim() && (profile.useCasesZh?.length ?? 0) > 0);
}

type SkillDetailProps = {
  skill?: skillmgr.Skill;
  syncConfigured: boolean;
  llmConfig?: skillmgr.SyncLLMConfig;
  isGeneratingProfile: boolean;
  onResolve: (skillId: string) => Promise<void>;
  onGenerateProfile: (skillId: string, force?: boolean) => Promise<void>;
  onReadEnv: (skillId: string) => Promise<string>;
  onSaveEnv: (skillId: string, content: string) => Promise<void>;
  onSaveTags: (skillId: string, tags: string[]) => Promise<void>;
  onListFiles: (skillId: string, relativeDir: string) => Promise<skillmgr.SkillFileEntry[]>;
  onReadFilePreview: (skillId: string, relativeFile: string) => Promise<skillmgr.SkillFilePreview>;
};

type DetailPreviewState = {
  path: string;
  content: string;
  previewable: boolean;
  loading: boolean;
  reason?: string;
};

function initialDetailPreview(skill: skillmgr.Skill): DetailPreviewState {
  return {
    path: skill.previewFile || "SKILL.md",
    content: skill.preview || "",
    previewable: Boolean(skill.preview),
    loading: false,
    reason: skill.preview ? undefined : "No text preview is available for this file.",
  };
}

function SkillDetail(props: SkillDetailProps) {
  if (!props.skill) {
    return <div className="p-5 text-sm text-muted-foreground">No skill selected.</div>;
  }
  return <SkillDetailContent {...props} skill={props.skill} />;
}

function SkillDetailContent({
  skill,
  syncConfigured,
  llmConfig,
  isGeneratingProfile,
  onResolve,
  onGenerateProfile,
  onReadEnv,
  onSaveEnv,
  onSaveTags,
  onListFiles,
  onReadFilePreview,
}: Omit<SkillDetailProps, "skill"> & { skill: skillmgr.Skill }) {
  const [preview, setPreview] = useState<DetailPreviewState>(() => initialDetailPreview(skill));
  const previewRequestRef = useRef(0);
  const activeLinkedPaths = linkedSkillPaths(skill);

  useEffect(() => {
    previewRequestRef.current += 1;
    setPreview(initialDetailPreview(skill));
  }, [skill.id, skill.preview, skill.previewFile]);

  async function selectPreviewFile(relativeFile: string) {
    const requestId = previewRequestRef.current + 1;
    previewRequestRef.current = requestId;
    if (relativeFile === skill.previewFile && skill.preview) {
      setPreview(initialDetailPreview(skill));
      return;
    }
    setPreview({ path: relativeFile, content: "", previewable: false, loading: true });
    try {
      const result = await onReadFilePreview(skill.id, relativeFile);
      if (previewRequestRef.current !== requestId) return;
      setPreview({
        path: result.path || relativeFile,
        content: result.content || "",
        previewable: result.previewable,
        loading: false,
        reason: result.reason,
      });
    } catch (error) {
      if (previewRequestRef.current !== requestId) return;
      setPreview({
        path: relativeFile,
        content: "",
        previewable: false,
        loading: false,
        reason: error instanceof Error ? error.message : String(error),
      });
    }
  }

  return (
    <div className="min-h-0 min-w-0 flex-1 overflow-y-auto p-5">
      <div className="mb-5 flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="truncate text-xl font-semibold">{skill.name}</h2>
          <p className="mt-1 break-words text-sm text-muted-foreground">{skill.description || skill.sourceAlias}</p>
        </div>
        <div className="shrink-0">
          <StatusPill status={skill.status} />
        </div>
      </div>

      <DetailSection title="Paths">
        {skill.sourcePath ? (
          <PathRow label="source skill folder" path={skill.sourcePath} />
        ) : (
          <PathRow label="source locator" path={skill.syncId || skill.repoId || skill.name} />
        )}
        {activeLinkedPaths.length > 0 && <PathRow label="currently linked to" path={activeLinkedPaths} />}
      </DetailSection>

      <SourceSection skill={skill} />

      <SkillProfileSections
        skill={skill}
        syncConfigured={syncConfigured}
        llmConfig={llmConfig}
        isGenerating={isGeneratingProfile}
        onGenerateProfile={onGenerateProfile}
      />

      <DetailSection title="Tags">
        <TagEditor skill={skill} onSaveTags={onSaveTags} />
      </DetailSection>

      <ManifestSection manifest={skill.manifest} />

      {(skill.validationErrors?.length || skill.error) && (
        <DetailSection title="Issues">
          {skill.error && <IssueLine value={skill.error} />}
          {(skill.validationErrors ?? []).map((issue) => (
            <IssueLine key={issue} value={issue} />
          ))}
        </DetailSection>
      )}

      {skill.status === "conflict" && (
        <DetailSection title="Conflict">
          <div className="space-y-2">
            {(skill.conflictSources ?? []).map((source) => (
              <button
                key={source.skillId}
                title={
                  source.skillId === skill.id
                    ? "Resolve this conflict by making this source the active skill target."
                    : "This source conflicts with the selected skill target."
                }
                className={cn(
                  "flex w-full items-center justify-between gap-2 rounded-md border p-2 text-left text-xs hover:bg-slate-50",
                  source.skillId === skill.id ? "border-blue-300 bg-blue-50" : "border-border",
                )}
                onClick={() => source.skillId === skill.id && onResolve(skill.id)}
              >
                <span className="min-w-0 truncate">{source.sourcePath}</span>
                <ChevronRight aria-hidden="true" className="h-4 w-4 shrink-0" />
              </button>
            ))}
            {(skill.conflictSources ?? []).length === 0 && (
              <div className="space-y-2">
                <IssueLine value={skill.symlinkTarget ? `Target currently points to ${skill.symlinkTarget}` : "Target name is already occupied."} />
                <Button
                  variant="outline"
                  onClick={() => void onResolve(skill.id)}
                  title="Replace the existing target link with this skill's source folder."
                >
                  Replace Existing Target
                </Button>
              </div>
            )}
          </div>
        </DetailSection>
      )}

      <DetailSection title="Files">
        <SkillFileTree
          skill={skill}
          onListFiles={onListFiles}
          onSelectFile={selectPreviewFile}
          selectedFile={preview.path}
        />
      </DetailSection>

      <DetailSection title={`Preview: ${preview.path}`}>
        {preview.loading ? (
          <div className="preview-message flex min-h-24 items-center gap-2 rounded-md border border-border bg-slate-50 p-3 text-xs text-muted-foreground">
            <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
            Reading text preview...
          </div>
        ) : preview.previewable ? (
          <pre className="code-preview code-preview--wrap max-h-72 max-w-full overflow-auto rounded-md border border-border bg-slate-950 p-3 text-xs leading-5 text-slate-100">
            {preview.content}
          </pre>
        ) : (
          <div className="preview-message min-h-24 rounded-md border border-border bg-slate-50 p-3 text-xs leading-5 text-muted-foreground">
            {preview.reason || "This file cannot be previewed as text."}
          </div>
        )}
      </DetailSection>

      <EnvEditor skill={skill} onReadEnv={onReadEnv} onSaveEnv={onSaveEnv} />
    </div>
  );
}

type FileDirectoryState = {
  entries: skillmgr.SkillFileEntry[];
  loading: boolean;
  loaded: boolean;
};

function SkillFileTree({
  skill,
  onListFiles,
  onSelectFile,
  selectedFile,
}: {
  skill: skillmgr.Skill;
  onListFiles: (skillId: string, relativeDir: string) => Promise<skillmgr.SkillFileEntry[]>;
  onSelectFile: (relativeFile: string) => void;
  selectedFile: string;
}) {
  const [directories, setDirectories] = useState<Record<string, FileDirectoryState>>({});
  const [openDirs, setOpenDirs] = useState<Set<string>>(() => new Set());
  const skillIdRef = useRef(skill.id);
  const rootState = directories[""];

  useEffect(() => {
    skillIdRef.current = skill.id;
    setOpenDirs(new Set());
    setDirectories({ "": { entries: [], loading: true, loaded: false } });
    let cancelled = false;
    onListFiles(skill.id, "").then((entries) => {
      if (cancelled || skillIdRef.current !== skill.id) return;
      setDirectories({ "": { entries, loading: false, loaded: true } });
    });
    return () => {
      cancelled = true;
    };
  }, [onListFiles, skill.id]);

  async function loadDirectory(relativeDir: string) {
    const requestSkillId = skill.id;
    setDirectories((current) => ({
      ...current,
      [relativeDir]: {
        entries: current[relativeDir]?.entries ?? [],
        loaded: current[relativeDir]?.loaded ?? false,
        loading: true,
      },
    }));
    const entries = await onListFiles(requestSkillId, relativeDir);
    if (skillIdRef.current !== requestSkillId) return;
    setDirectories((current) => ({
      ...current,
      [relativeDir]: { entries, loading: false, loaded: true },
    }));
  }

  function toggleDirectory(relativeDir: string) {
    const nextOpen = !openDirs.has(relativeDir);
    setOpenDirs((current) => {
      const next = new Set(current);
      if (nextOpen) {
        next.add(relativeDir);
      } else {
        next.delete(relativeDir);
      }
      return next;
    });
    if (nextOpen && !directories[relativeDir]?.loaded && !directories[relativeDir]?.loading) {
      void loadDirectory(relativeDir);
    }
  }

  return (
    <div className="file-tree-card min-w-0 overflow-hidden rounded-md border border-border bg-white">
      {rootState?.loading ? (
        <FileTreeMessage>Loading files...</FileTreeMessage>
      ) : rootState?.entries.length ? (
        <FileTreeEntries
          depth={0}
          directories={directories}
          entries={rootState.entries}
          onSelectFile={onSelectFile}
          onToggleDirectory={toggleDirectory}
          openDirs={openDirs}
          selectedFile={selectedFile}
        />
      ) : (
        <FileTreeMessage>No files found.</FileTreeMessage>
      )}
    </div>
  );
}

function FileTreeEntries({
  entries,
  depth,
  directories,
  openDirs,
  selectedFile,
  onSelectFile,
  onToggleDirectory,
}: {
  entries: skillmgr.SkillFileEntry[];
  depth: number;
  directories: Record<string, FileDirectoryState>;
  openDirs: Set<string>;
  selectedFile: string;
  onSelectFile: (relativeFile: string) => void;
  onToggleDirectory: (relativeDir: string) => void;
}) {
  return (
    <div className="file-tree-branch min-w-0">
      {entries.map((entry) => {
        const isOpen = openDirs.has(entry.path);
        const directoryState = directories[entry.path];
        const indent = `${0.5 + depth * 0.85}rem`;
        if (!entry.isDir) {
          return (
            <button
              key={entry.path}
              aria-pressed={selectedFile === entry.path}
              className={cn(
                "file-tree-row file-tree-row--button min-w-0",
                selectedFile === entry.path && "file-tree-row--selected",
              )}
              onClick={() => onSelectFile(entry.path)}
              style={{ paddingLeft: indent }}
              title={`Load ${entry.path} into the text preview panel when its content is previewable.`}
              type="button"
            >
              <span className="file-tree-spacer" aria-hidden="true" />
              <FileText aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-slate-500" />
              <span className="min-w-0 truncate">{entry.name}</span>
            </button>
          );
        }
        return (
          <div key={entry.path} className="min-w-0">
            <button
              aria-expanded={isOpen}
              className="file-tree-row file-tree-row--button min-w-0"
              onClick={() => onToggleDirectory(entry.path)}
              style={{ paddingLeft: indent }}
              title={isOpen ? `Collapse the ${entry.path} folder.` : `Load and show files inside the ${entry.path} folder.`}
              type="button"
            >
              {isOpen ? (
                <ChevronDown aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <ChevronRight aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
              )}
              {isOpen ? (
                <FolderOpen aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-[var(--sm-conflict)]" />
              ) : (
                <Folder aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-[var(--sm-conflict)]" />
              )}
              <span className="min-w-0 truncate">{entry.name}</span>
            </button>
            {isOpen && directoryState?.loading && (
              <FileTreeMessage depth={depth + 1}>Loading files...</FileTreeMessage>
            )}
            {isOpen && directoryState?.loaded && directoryState.entries.length === 0 && (
              <FileTreeMessage depth={depth + 1}>Empty folder.</FileTreeMessage>
            )}
            {isOpen && directoryState?.entries.length ? (
              <FileTreeEntries
                depth={depth + 1}
                directories={directories}
                entries={directoryState.entries}
                onSelectFile={onSelectFile}
                onToggleDirectory={onToggleDirectory}
                openDirs={openDirs}
                selectedFile={selectedFile}
              />
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function FileTreeMessage({ children, depth = 0 }: { children: ReactNode; depth?: number }) {
  return (
    <div className="file-tree-message truncate px-3 py-2 text-xs text-muted-foreground" style={{ paddingLeft: `${0.75 + depth * 0.85}rem` }}>
      {children}
    </div>
  );
}

function SourceSection({ skill }: { skill: skillmgr.Skill }) {
  return (
    <DetailSection title="Source">
      <div className="min-w-0 space-y-2 rounded-md border border-border bg-slate-50 p-3 text-xs">
        {skill.repoId && (
          <div className="grid min-w-0 grid-cols-[minmax(0,96px)_minmax(0,1fr)] gap-2">
            <span className="text-muted-foreground">repo</span>
            <span className="break-all font-mono">{skill.repoId}</span>
          </div>
        )}
        {skill.repoSubpath && (
          <div className="grid min-w-0 grid-cols-[minmax(0,96px)_minmax(0,1fr)] gap-2">
            <span className="text-muted-foreground">subpath</span>
            <span className="break-all font-mono">{skill.repoSubpath}</span>
          </div>
        )}
        {skill.ref && (
          <div className="grid min-w-0 grid-cols-[minmax(0,96px)_minmax(0,1fr)] gap-2">
            <span className="text-muted-foreground">ref</span>
            <span className={cn("break-all font-mono", skill.refMismatch && "text-amber-700")}>
              {skill.ref}
              {skill.refMismatch ? " mismatch" : ""}
            </span>
          </div>
        )}
      </div>
    </DetailSection>
  );
}

function SkillProfileSections({
  skill,
  syncConfigured,
  llmConfig,
  isGenerating,
  onGenerateProfile,
}: {
  skill: skillmgr.Skill;
  syncConfigured: boolean;
  llmConfig?: skillmgr.SyncLLMConfig;
  isGenerating: boolean;
  onGenerateProfile: (skillId: string, force?: boolean) => Promise<void>;
}) {
  const profile = skill.profile;
  const canGenerate = Boolean(syncConfigured && skill.syncId && skill.sourcePath && llmConfigReady(llmConfig));
  const hasProfile = profileComplete(profile);
  const emptyMessage = isGenerating ? "正在生成..." : llmConfigReady(llmConfig) ? "尚未生成。" : "未配置 LLM。";

  return (
    <>
      <DetailSection title="能力简介">
        <div className="min-w-0 rounded-md border border-border bg-slate-50 p-3">
          {profile?.summaryZh ? (
            <p className="break-words text-sm leading-6 text-slate-800">{profile.summaryZh}</p>
          ) : (
            <p className="text-sm text-muted-foreground">{emptyMessage}</p>
          )}
          <div className="mt-3 flex min-w-0 flex-wrap items-center justify-between gap-2">
            <span className="min-w-0 truncate text-xs text-muted-foreground">
              {profile?.generatedAt
                ? `${profile.model || "LLM"} · ${formatDate(profile.generatedAt)}`
                : skill.syncId || skill.repoSubpath || skill.name}
            </span>
            <Button
              variant="outline"
              onClick={() => void onGenerateProfile(skill.id, true)}
              disabled={!canGenerate || isGenerating}
              title={
                canGenerate
                  ? "Ask the configured LLM to regenerate the Chinese summary and use cases for this skill."
                  : "Configure sync and LLM settings before generating this skill profile."
              }
            >
              {isGenerating ? (
                <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCcw aria-hidden="true" className="h-4 w-4" />
              )}
              {isGenerating ? "生成中..." : hasProfile ? "重新生成" : "生成"}
            </Button>
          </div>
        </div>
      </DetailSection>

      <DetailSection title="使用案例">
        <div className="min-w-0 rounded-md border border-border bg-white p-3">
          {profile?.useCasesZh?.length ? (
            <ul className="space-y-2">
              {profile.useCasesZh.map((useCase) => (
                <li key={useCase} className="flex min-w-0 gap-2 text-sm leading-6 text-slate-800">
                  <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-500" />
                  <span className="min-w-0 break-words">{useCase}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">{emptyMessage}</p>
          )}
        </div>
      </DetailSection>
    </>
  );
}

function ManifestSection({ manifest }: { manifest?: skillmgr.SkillManifest }) {
  if (!manifest) {
    return null;
  }
  const metadataEntries = Object.entries(manifest.metadata ?? {});
  return (
    <DetailSection title="Manifest">
      <div className="manifest-card min-w-0 space-y-2 rounded-md border border-border bg-slate-50 p-3">
        <ManifestRow label="name" value={manifest.name} />
        <ManifestRow label="description" value={manifest.description} />
        <ManifestRow label="license" value={manifest.license} />
        <ManifestRow label="compatibility" value={manifest.compatibility} />
        <ManifestRow label="allowedTools" value={manifest.allowedTools} />
        <ManifestRow label="whenToUse" value={manifest.whenToUse} />
        <ManifestRow
          label="disableModelInvocation"
          value={manifest.disableModelInvocation === undefined ? undefined : String(manifest.disableModelInvocation)}
        />
        <ManifestRow
          label="userInvocable"
          value={manifest.userInvocable === undefined ? undefined : String(manifest.userInvocable)}
        />
        <ManifestRow label="argumentHint" value={manifest.argumentHint} />
        <ManifestRow label="arguments" value={formatManifestArguments(manifest.arguments)} />
        {metadataEntries.length > 0 && (
          <div className="border-t border-border pt-2">
            <div className="mb-1 text-xs font-medium text-muted-foreground">metadata</div>
            <div className="space-y-1">
              {metadataEntries.map(([key, value]) => (
                <div key={key} className="grid min-w-0 grid-cols-[minmax(0,96px)_minmax(0,1fr)] gap-2 text-xs">
                  <span className="truncate font-mono text-slate-500">{key}</span>
                  <span className="break-words text-slate-700">{value}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </DetailSection>
  );
}

function TagEditor({
  skill,
  onSaveTags,
}: {
  skill: skillmgr.Skill;
  onSaveTags: (skillId: string, tags: string[]) => Promise<void>;
}) {
  const [draft, setDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const tags = skill.tags ?? [];

  useEffect(() => {
    setDraft("");
  }, [skill.name]);

  async function save(nextTags: string[]) {
    setSaving(true);
    try {
      await onSaveTags(skill.id, cleanUiTags(nextTags));
      setDraft("");
    } finally {
      setSaving(false);
    }
  }

  async function addDraftTag() {
    const nextTags = cleanUiTags([...tags, ...draft.split(",")]);
    if (nextTags.length === tags.length && nextTags.every((tag, index) => tag === tags[index])) {
      setDraft("");
      return;
    }
    await save(nextTags);
  }

  return (
    <div className="tag-editor min-w-0 rounded-md border border-border bg-white p-3">
      <TagList tags={tags} onRemove={(tag) => save(tags.filter((item) => item !== tag))} disabled={saving} />
      <div className="mt-3 flex min-w-0 gap-2">
        <div className="relative min-w-0 flex-1">
          <Tag aria-hidden="true" className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <input
            aria-label={`Add tag to ${skill.name}`}
            autoComplete="off"
            name={`${skill.name}-tag`}
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                void addDraftTag();
              }
            }}
            placeholder="Add tag…"
            className="h-9 w-full rounded-md border border-input bg-white pl-8 pr-3 text-sm"
          />
        </div>
        <Button
          variant="outline"
          onClick={() => void addDraftTag()}
          disabled={saving || !draft.trim()}
          title="Save the typed tag to this skill, splitting comma-separated entries."
        >
          {saving ? "Saving…" : "Add"}
        </Button>
      </div>
    </div>
  );
}

function TagList({
  tags,
  compact = false,
  disabled = false,
  onRemove,
  trailing,
}: {
  tags?: string[];
  compact?: boolean;
  disabled?: boolean;
  onRemove?: (tag: string) => void;
  trailing?: ReactNode;
}) {
  const visibleTags = tags ?? [];
  if (visibleTags.length === 0 && !trailing) {
    if (compact) return null;
    return <div className="text-xs text-muted-foreground">No tags yet.</div>;
  }
  return (
    <div className={cn("tag-list flex min-w-0 flex-wrap gap-1.5", compact && "mt-1")}>
      {visibleTags.map((tag) => (
        <span
          key={tag}
          className="tag-chip inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-0.5 text-xs"
          style={tagToneStyle(tag)}
        >
          <span className="truncate">{tag}</span>
          {onRemove && (
            <button
              aria-label={`Remove ${tag} tag`}
              title={`Remove the ${tag} tag from this skill.`}
              className="rounded p-0.5 hover:bg-rose-100 disabled:pointer-events-none disabled:opacity-50"
              disabled={disabled}
              onClick={() => onRemove(tag)}
              type="button"
            >
              <X aria-hidden="true" className="h-3 w-3" />
            </button>
          )}
        </span>
      ))}
      {trailing}
    </div>
  );
}

function SkillTagsCell({
  skill,
  onOpenPicker,
}: {
  skill: skillmgr.Skill;
  onOpenPicker: (anchor: HTMLElement) => void;
}) {
  return (
    <TagList
      tags={skill.tags}
      trailing={
        <button
          aria-label={`Add a tag to ${skill.displayName || skill.name}`}
          className="tag-add-button inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-dashed border-slate-300 bg-white text-slate-500 hover:border-[var(--sm-link)] hover:text-[var(--sm-link)]"
          onClick={(event) => {
            event.stopPropagation();
            onOpenPicker(event.currentTarget);
          }}
          onKeyDown={(event) => event.stopPropagation()}
          title="Search existing tags or create a new tag, then add it to this skill immediately."
          type="button"
        >
          <Plus aria-hidden="true" className="h-3.5 w-3.5" />
        </button>
      }
    />
  );
}

function TagPickerPopover({
  allTags,
  anchor,
  existingTags,
  mode,
  selectedCount,
  onClose,
  onSingleSelect,
  onBulkSubmit,
}: {
  allTags: string[];
  anchor: DOMRect;
  existingTags: string[];
  mode: "single" | "bulk";
  selectedCount: number;
  onClose: () => void;
  onSingleSelect: (tag: string) => Promise<void>;
  onBulkSubmit: (tags: string[]) => Promise<void>;
}) {
  const [query, setQuery] = useState("");
  const [pendingTags, setPendingTags] = useState<string[]>([]);
  const popoverRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const existingSet = useMemo(() => new Set(existingTags), [existingTags]);
  const normalizedQuery = query.trim();
  const filteredTags = allTags.filter((tag) => {
    if (mode === "single" && existingSet.has(tag)) return false;
    return !normalizedQuery || tag.toLowerCase().includes(normalizedQuery.toLowerCase());
  });
  const exactTagExists = allTags.some((tag) => tag.toLowerCase() === normalizedQuery.toLowerCase());
  const canCreate = Boolean(normalizedQuery) && !exactTagExists && !existingSet.has(normalizedQuery);
  const width = Math.min(300, window.innerWidth - 24);
  const estimatedHeight = mode === "bulk" ? 390 : 320;
  const left = Math.max(12, Math.min(anchor.left, window.innerWidth - width - 12));
  const top =
    anchor.bottom + estimatedHeight <= window.innerHeight - 12
      ? anchor.bottom + 6
      : Math.max(12, anchor.top - estimatedHeight - 6);

  useEffect(() => {
    inputRef.current?.focus();
    function closeOnOutsidePointer(event: MouseEvent) {
      if (!popoverRef.current?.contains(event.target as Node)) onClose();
    }
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    function closeOnViewportChange() {
      onClose();
    }
    document.addEventListener("mousedown", closeOnOutsidePointer);
    document.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", closeOnViewportChange);
    window.addEventListener("scroll", closeOnViewportChange, true);
    return () => {
      document.removeEventListener("mousedown", closeOnOutsidePointer);
      document.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", closeOnViewportChange);
      window.removeEventListener("scroll", closeOnViewportChange, true);
    };
  }, [onClose]);

  function togglePendingTag(tag: string) {
    setPendingTags((current) =>
      current.includes(tag)
        ? current.filter((item) => item !== tag)
        : cleanUiTags([...current, tag]),
    );
  }

  function chooseTag(tag: string) {
    if (mode === "single") {
      void onSingleSelect(tag);
      return;
    }
    togglePendingTag(tag);
    setQuery("");
  }

  function submitTypedTag() {
    if (!normalizedQuery || existingSet.has(normalizedQuery)) return;
    if (mode === "single") {
      void onSingleSelect(normalizedQuery);
      return;
    }
    setPendingTags((current) => cleanUiTags([...current, normalizedQuery]));
    setQuery("");
  }

  return createPortal(
    <div
      ref={popoverRef}
      className="tag-picker fixed z-[80] overflow-hidden rounded-md border border-border bg-white shadow-xl"
      style={{ left, top, width }}
      role="dialog"
      aria-label={mode === "single" ? "Add tag to skill" : "Add tags to selected skills"}
    >
      <div className="border-b border-border p-2">
        <div className="relative">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <input
            ref={inputRef}
            aria-label="Search or create tags"
            autoComplete="off"
            className="h-9 w-full rounded-md border border-input bg-white pl-8 pr-3 text-sm"
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                submitTypedTag();
              }
            }}
            placeholder="Search or create a tag..."
            value={query}
          />
        </div>
      </div>
      {mode === "bulk" && pendingTags.length > 0 && (
        <div className="flex max-h-20 flex-wrap gap-1.5 overflow-auto border-b border-border bg-slate-50 p-2">
          {pendingTags.map((tag) => (
            <button
              key={tag}
              className="tag-chip inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-0.5 text-xs"
              onClick={() => togglePendingTag(tag)}
              style={tagToneStyle(tag)}
              title={`Remove ${tag} from the tags waiting to be added.`}
              type="button"
            >
              <span className="truncate">{tag}</span>
              <X aria-hidden="true" className="h-3 w-3" />
            </button>
          ))}
        </div>
      )}
      <div className="max-h-56 overflow-y-auto py-1">
        {canCreate && (
          <button
            className="tag-picker-option flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-cyan-50"
            onClick={submitTypedTag}
            type="button"
          >
            <Plus aria-hidden="true" className="h-4 w-4 text-[var(--sm-link)]" />
            <span className="min-w-0 truncate">Create “{normalizedQuery}”</span>
          </button>
        )}
        {filteredTags.map((tag) => {
          const pending = pendingTags.includes(tag);
          return (
            <button
              key={tag}
              className="tag-picker-option flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-cyan-50"
              onClick={() => chooseTag(tag)}
              type="button"
            >
              <span className="tag-chip min-w-0 truncate rounded-md border px-2 py-0.5 text-xs" style={tagToneStyle(tag)}>
                {tag}
              </span>
              {mode === "bulk" && pending && <Check aria-hidden="true" className="h-4 w-4 shrink-0 text-[var(--sm-enabled)]" />}
            </button>
          );
        })}
        {!canCreate && filteredTags.length === 0 && (
          <div className="px-3 py-4 text-center text-xs text-muted-foreground">
            {mode === "single" ? "No additional tags match." : "No tags match this search."}
          </div>
        )}
      </div>
      {mode === "bulk" && (
        <div className="flex items-center justify-between gap-3 border-t border-border bg-slate-50 p-2">
          <span className="truncate text-xs text-muted-foreground">
            {pendingTags.length} {pendingTags.length === 1 ? "tag" : "tags"} selected
          </span>
          <Button
            disabled={pendingTags.length === 0 || selectedCount === 0}
            onClick={() => void onBulkSubmit(pendingTags)}
            title="Append the selected tags to every selected skill, keeping all tags already assigned."
          >
            Add to {selectedCount} {selectedCount === 1 ? "Skill" : "Skills"}
          </Button>
        </div>
      )}
    </div>,
    document.body,
  );
}

function TagFilterButton({
  tag,
  selected,
  onClick,
}: {
  tag: string;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      aria-pressed={selected}
      className={cn(
        "tag-filter-chip inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium transition",
        selected && "tag-filter-chip--active",
      )}
      onClick={onClick}
      style={tagToneStyle(tag)}
      title={
        selected
          ? `Remove the ${tag} tag from the active skill filter.`
          : `Show skills that have the ${tag} tag; multiple selected tags match any tag.`
      }
      type="button"
    >
      <Tag aria-hidden="true" className="h-3 w-3 shrink-0" />
      <span className="truncate">{tag}</span>
    </button>
  );
}

function ManifestRow({ label, value }: { label: string; value?: string }) {
  if (!value) {
    return null;
  }
  return (
    <div className="grid min-w-0 grid-cols-[minmax(0,112px)_minmax(0,1fr)] gap-2 text-xs">
      <span className="truncate font-mono text-slate-500">{label}</span>
      <span className="break-words text-slate-700">{value}</span>
    </div>
  );
}

function formatManifestArguments(value: unknown) {
  if (value === undefined || value === null) {
    return undefined;
  }
  if (Array.isArray(value)) {
    return value.join(", ");
  }
  return String(value);
}

function EnvEditor({
  skill,
  onReadEnv,
  onSaveEnv,
}: {
  skill: skillmgr.Skill;
  onReadEnv: (skillId: string) => Promise<string>;
  onSaveEnv: (skillId: string, content: string) => Promise<void>;
}) {
  const [content, setContent] = useState("");
  const [savedContent, setSavedContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const dirty = content !== savedContent;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    onReadEnv(skill.id)
      .then((value) => {
        if (cancelled) return;
        setContent(value);
        setSavedContent(value);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [onReadEnv, skill.id]);

  async function save() {
    setSaving(true);
    try {
      await onSaveEnv(skill.id, content);
      setSavedContent(content);
    } finally {
      setSaving(false);
    }
  }

  async function reload() {
    setLoading(true);
    const value = await onReadEnv(skill.id);
    setContent(value);
    setSavedContent(value);
    setLoading(false);
  }

  return (
    <DetailSection title=".env overrides">
      <div className="env-editor min-w-0 rounded-md border border-border bg-white">
        <textarea
          aria-label={`${skill.name} .env overrides`}
          autoComplete="off"
          name={`${skill.id}-env-overrides`}
          value={content}
          onChange={(event) => setContent(event.target.value)}
          spellCheck={false}
          placeholder="KEY=value…"
          className="code-preview min-h-40 min-w-0 w-full resize-y border-0 bg-slate-950 p-3 font-mono text-xs leading-5 text-slate-100 outline-none placeholder:text-slate-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
        />
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 border-t border-border bg-slate-50 px-3 py-2">
          <span className="text-xs text-muted-foreground" aria-live="polite">
            {loading ? "Reading .env…" : dirty ? "Unsaved changes" : ".env saved"}
          </span>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={reload}
              disabled={loading || saving}
              title="Reload this skill's .env overrides from disk and discard unsaved editor text."
            >
              Reload
            </Button>
            <Button
              onClick={save}
              disabled={loading || saving || !dirty}
              title="Write the current .env override text to this skill's local override file."
            >
              {saving ? "Saving…" : "Save .env"}
            </Button>
          </div>
        </div>
      </div>
    </DetailSection>
  );
}

function SettingsModal({
  inventory,
  onClose,
  onBrowseTarget,
  onBrowseSyncFolder,
  onSave,
}: {
  inventory: skillmgr.Inventory;
  onClose: () => void;
  onBrowseTarget: () => Promise<string>;
  onBrowseSyncFolder: () => Promise<string>;
  onSave: (config: skillmgr.Config, llmConfig: skillmgr.SyncLLMConfig) => Promise<void>;
}) {
  const [config, setConfig] = useState(() => skillmgr.Config.createFrom(inventory.config));
  const [llmConfig, setLLMConfig] = useState(() => skillmgr.SyncLLMConfig.createFrom(inventory.llmConfig ?? {}));
  const [newTargetDir, setNewTargetDir] = useState("");
  const updateConfig = (next: Partial<skillmgr.Config>) => {
    setConfig(skillmgr.Config.createFrom({ ...config, ...next }));
  };
  const updateLLMConfig = (next: Partial<skillmgr.SyncLLMConfig>) => {
    setLLMConfig(skillmgr.SyncLLMConfig.createFrom({ ...llmConfig, ...next }));
  };
  const updateScan = (next: Partial<skillmgr.ScanConfig>) => {
    updateConfig({ scan: skillmgr.ScanConfig.createFrom({ ...config.scan, ...next }) });
  };
  const updateSync = (next: Partial<skillmgr.SyncConfig>) => {
    updateConfig({ sync: skillmgr.SyncConfig.createFrom({ ...(config.sync ?? {}), ...next }) });
  };
  const targetDirs = config.targetDirs?.length ? config.targetDirs : ["~/.agents/skills"];
  const updateTargetDir = (index: number, value: string) => {
    updateConfig({ targetDirs: targetDirs.map((targetDir, itemIndex) => (itemIndex === index ? value : targetDir)) });
  };
  const removeTargetDir = (index: number) => {
    updateConfig({ targetDirs: targetDirs.filter((_, itemIndex) => itemIndex !== index) });
  };
  const addTargetDir = async () => {
    const trimmed = newTargetDir.trim();
    const targetDir = trimmed || (await onBrowseTarget());
    if (!targetDir) return;
    updateConfig({ targetDirs: [...targetDirs, targetDir] });
    setNewTargetDir("");
  };

  return (
    <Modal title="Settings" onClose={onClose}>
      <div className="space-y-4">
        <div className="block text-sm font-medium">
          Target skill directories
          <div className="mt-2 space-y-2">
            {targetDirs.map((targetDir, index) => (
              <div key={`${targetDir}-${index}`} className="flex gap-2">
                <input
                  aria-label={`Target skill directory ${index + 1}`}
                  autoComplete="off"
                  name={`target-directory-${index}`}
                  value={targetDir}
                  onChange={(event) => updateTargetDir(index, event.target.value)}
                  className="h-9 min-w-0 flex-1 rounded-md border border-input px-3 text-sm"
                />
                <IconButton
                  title="Remove this target directory from settings; existing files are not deleted."
                  onClick={() => removeTargetDir(index)}
                >
                  <X aria-hidden="true" className="h-4 w-4" />
                </IconButton>
              </div>
            ))}
            <div className="flex gap-2">
              <input
                aria-label="New target skill directory"
                autoComplete="off"
                name="new-target-directory"
                value={newTargetDir}
                onChange={(event) => setNewTargetDir(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    void addTargetDir();
                  }
                }}
                className="h-9 min-w-0 flex-1 rounded-md border border-input px-3 text-sm"
                placeholder="/Users/yusuf/.agents/skills…"
              />
              <Button
                variant="outline"
                onClick={() => void addTargetDir()}
                title="Add the typed folder, or open a folder picker if the field is empty."
              >
                Add
              </Button>
            </div>
          </div>
        </div>
        <div className="block text-sm font-medium">
          iCloud sync folder
          <div className="mt-2 flex gap-2">
            <input
              aria-label="iCloud sync folder"
              autoComplete="off"
              name="sync-folder"
              value={config.sync?.folder ?? ""}
              onChange={(event) => updateSync({ folder: event.target.value })}
              className="h-9 min-w-0 flex-1 rounded-md border border-input px-3 text-sm"
              placeholder="/Users/me/Library/Mobile Documents/com~apple~CloudDocs/SkillManager"
            />
            <Button
              variant="outline"
              title="Open a folder picker and use the selected folder for iCloud sync."
              onClick={async () => {
                const folder = await onBrowseSyncFolder();
                if (folder) updateSync({ folder });
              }}
            >
              Browse
            </Button>
          </div>
          {inventory.syncPath && <div className="mt-2 break-all font-mono text-xs text-muted-foreground">{inventory.syncPath}</div>}
        </div>
        <div className="block text-sm font-medium">
          LLM profile generation
          <div className="mt-2 space-y-3 rounded-md border border-border bg-slate-50 p-3">
            <label className="block text-xs font-medium text-slate-700">
              Base URL
              <input
                aria-label="LLM base URL"
                autoComplete="off"
                name="llm-base-url"
                value={llmConfig.baseUrl ?? ""}
                onChange={(event) => updateLLMConfig({ baseUrl: event.target.value })}
                className="mt-1 h-9 w-full rounded-md border border-input bg-white px-3 text-sm"
                placeholder="https://api.deepseek.com"
              />
            </label>
            <label className="block text-xs font-medium text-slate-700">
              API Key
              <input
                aria-label="LLM API key"
                autoComplete="off"
                name="llm-api-key"
                type="password"
                value={llmConfig.apiKey ?? ""}
                onChange={(event) => updateLLMConfig({ apiKey: event.target.value })}
                className="mt-1 h-9 w-full rounded-md border border-input bg-white px-3 text-sm"
                placeholder="sk-..."
              />
            </label>
            <label className="block text-xs font-medium text-slate-700">
              Model
              <input
                aria-label="LLM model"
                autoComplete="off"
                name="llm-model"
                value={llmConfig.model ?? ""}
                onChange={(event) => updateLLMConfig({ model: event.target.value })}
                className="mt-1 h-9 w-full rounded-md border border-input bg-white px-3 text-sm"
                placeholder="deepseek-v4-flash"
              />
            </label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="block text-xs font-medium text-slate-700">
                Temperature
                <input
                  aria-label="LLM temperature"
                  autoComplete="off"
                  min={0}
                  name="llm-temperature"
                  step={0.1}
                  type="number"
                  value={llmConfig.temperature ?? 0}
                  onChange={(event) => updateLLMConfig({ temperature: Number(event.target.value) || 0 })}
                  className="mt-1 h-9 w-full rounded-md border border-input bg-white px-3 text-sm"
                />
              </label>
              <label className="block text-xs font-medium text-slate-700">
                Max tokens
                <input
                  aria-label="LLM max tokens"
                  autoComplete="off"
                  min={0}
                  name="llm-max-tokens"
                  step={100}
                  type="number"
                  value={llmConfig.maxTokens || ""}
                  onChange={(event) => updateLLMConfig({ maxTokens: Number(event.target.value) || 0 })}
                  className="mt-1 h-9 w-full rounded-md border border-input bg-white px-3 text-sm"
                  placeholder="1200"
                />
              </label>
            </div>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex items-center gap-2 rounded-md border border-border p-3 text-sm">
            <input
              name="auto-rescan-on-startup"
              type="checkbox"
              checked={config.scan.autoRescanOnStartup}
              onChange={(event) => updateScan({ autoRescanOnStartup: event.target.checked })}
            />
            Auto rescan on startup
          </label>
          <label className="flex items-center gap-2 rounded-md border border-border p-3 text-sm">
            <input
              name="watch-source-folders"
              type="checkbox"
              checked={config.scan.watchSourceFolders}
              onChange={(event) => updateScan({ watchSourceFolders: event.target.checked })}
            />
            Watch source folders
          </label>
        </div>
        <div className="block text-sm font-medium">
          Skill detection
          <div className="mt-2 rounded-md border border-border bg-slate-50 p-3 text-sm text-muted-foreground">
            A folder appears here when it contains `SKILL.md`.
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} title="Close settings without applying unsaved changes.">
            Cancel
          </Button>
          <Button title="Save target folders, sync location, scan options, and LLM settings." onClick={() => onSave(config, llmConfig)}>
            Save
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function CloneDestinationModal({
  source,
  parentDir,
  onClose,
  onClone,
}: {
  source: skillmgr.Repository;
  parentDir: string;
  onClose: () => void;
  onClone: (folderName: string) => Promise<boolean>;
}) {
  const [folderName, setFolderName] = useState(() => defaultRepoFolderName(source.repoId));
  const [cloning, setCloning] = useState(false);
  const trimmedName = folderName.trim();
  const validationError = cloneFolderNameError(trimmedName);
  const destination = trimmedName ? `${parentDir.replace(/[\\/]$/, "")}/${trimmedName}` : parentDir;

  async function clone() {
    if (validationError) return;
    setCloning(true);
    try {
      if (await onClone(trimmedName)) {
        onClose();
      }
    } finally {
      setCloning(false);
    }
  }

  return (
    <Modal title="Clone Repository" onClose={() => !cloning && onClose()}>
      <div className="space-y-4">
        <div className="grid min-w-0 grid-cols-[88px_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
          <span className="text-muted-foreground">Repository</span>
          <span className="truncate font-mono text-xs">{source.repoId}</span>
          <span className="text-muted-foreground">Parent</span>
          <span className="truncate font-mono text-xs">{parentDir}</span>
        </div>
        <label className="block text-sm font-medium">
          Folder name
          <input
            autoFocus
            autoComplete="off"
            name="clone-folder-name"
            value={folderName}
            onChange={(event) => setFolderName(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !validationError && !cloning) {
                event.preventDefault();
                void clone();
              }
            }}
            className="mt-2 h-9 w-full rounded-md border border-input px-3 text-sm"
          />
        </label>
        {validationError ? (
          <div className="text-xs text-rose-700">{validationError}</div>
        ) : (
          <div className="break-all border-t border-border pt-3 font-mono text-xs text-muted-foreground">{destination}</div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={cloning} title="Cancel without cloning this repository.">
            Cancel
          </Button>
          <Button
            onClick={() => void clone()}
            disabled={cloning || Boolean(validationError)}
            title="Clone this shallow repository into the displayed destination and register it on this machine."
          >
            {cloning ? "Cloning..." : "Clone"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function SourceAliasModal({
  source,
  onClose,
  onSave,
}: {
  source: RepositoryPanelItem;
  onClose: () => void;
  onSave: (alias: string) => Promise<void>;
}) {
  const [alias, setAlias] = useState(source.alias || "");
  const [saving, setSaving] = useState(false);

  async function save() {
    setSaving(true);
    try {
      await onSave(alias.trim());
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal title="Rename Alias" onClose={onClose}>
      <div className="space-y-4">
        <label className="block text-sm font-medium">
          Alias
          <input
            autoComplete="off"
            name="source-alias"
            value={alias}
            onChange={(event) => setAlias(event.target.value)}
            className="mt-2 h-9 w-full rounded-md border border-input px-3 text-sm"
            placeholder={`${basename(source.path)}…`}
          />
        </label>
        <div className="rounded-md border border-border bg-slate-50 p-3 font-mono text-xs text-muted-foreground">
          {source.path}
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={saving} title="Close this dialog without changing the source alias.">
            Cancel
          </Button>
          <Button title="Save this alias as the display name for the selected source." onClick={save} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function RemoveSourceModal({
  source,
  onClose,
  onRemove,
}: {
  source: RepositoryPanelItem;
  onClose: () => void;
  onRemove: () => Promise<void>;
}) {
  const [removing, setRemoving] = useState(false);

  async function remove() {
    setRemoving(true);
    try {
      await onRemove();
    } finally {
      setRemoving(false);
    }
  }

  return (
    <Modal title="Remove Repository" onClose={onClose}>
      <div className="space-y-4">
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          This only removes the local mapping from Skill Manager. Files, sync records, and existing symlinks will not be deleted.
        </div>
        <div>
          <div className="text-sm font-medium">{repositoryItemTitle(source)}</div>
          <div className="mt-1 break-all font-mono text-xs text-muted-foreground">{source.path}</div>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={removing} title="Close this dialog and keep the source mapped.">
            Cancel
          </Button>
          <Button title="Remove this source mapping from Skill Manager without deleting source files." onClick={remove} disabled={removing}>
            {removing ? "Removing…" : "Remove"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function StatusPill({ status }: { status: string }) {
  return (
    <span
      className={cn(
        "status-pill inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium",
        statusClass[status] ?? statusClass.disabled,
      )}
    >
      {status === "enabled" && <Check aria-hidden="true" className="h-3 w-3" />}
      {status === "conflict" && <AlertTriangle aria-hidden="true" className="h-3 w-3" />}
      {(status === "missing-source" || status === "missing-path") && <AlertTriangle aria-hidden="true" className="h-3 w-3" />}
      {statusLabels[status] ?? status}
    </span>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="detail-section mb-5 min-w-0">
      <h3 className="mb-2 truncate text-xs font-semibold tracking-normal text-muted-foreground">{title}</h3>
      {children}
    </section>
  );
}

function PathRow({ label, path }: { label: string; path: string | string[] }) {
  const paths = Array.isArray(path) ? path : [path];
  return (
    <div className="path-row mb-2 min-w-0 rounded-md border border-border p-2">
      <div className="mb-1 text-xs text-muted-foreground">{label}</div>
      <div className="space-y-1">
        {paths.map((item) => (
          <div key={item} className="break-all font-mono text-xs text-slate-700">
            {item}
          </div>
        ))}
      </div>
    </div>
  );
}

function IssueLine({ value }: { value: string }) {
  return (
    <div className="issue-line min-w-0 break-words rounded-md border border-rose-200 bg-rose-50 p-2 text-sm text-rose-700">{value}</div>
  );
}

function PanelHeader({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <div className="panel-header flex h-14 min-w-0 shrink-0 items-center justify-between gap-2 border-b border-border px-4">
      <h2 className="min-w-0 truncate text-sm font-semibold">{title}</h2>
      {children && <div className="shrink-0">{children}</div>}
    </div>
  );
}

function Button({
  variant = "default",
  className,
  children,
  title,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "default" | "outline" | "ghost" }) {
  return (
    <button
      className={cn(
        "ui-button inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition disabled:pointer-events-none disabled:opacity-50",
        variant === "default" && "ui-button--default bg-primary text-primary-foreground hover:bg-blue-600",
        variant === "outline" && "ui-button--outline border border-border bg-white text-foreground hover:bg-slate-50",
        variant === "ghost" && "ui-button--ghost hover:bg-slate-100",
        className,
      )}
      title={title}
      {...props}
    >
      {children}
    </button>
  );
}

function IconButton({
  className,
  title,
  "aria-label": ariaLabel,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const accessibleLabel = ariaLabel ?? (typeof title === "string" ? title : undefined);
  return (
    <button
      aria-label={accessibleLabel}
      className={cn(
        "icon-button inline-flex h-9 w-9 items-center justify-center rounded-md border border-border bg-white text-slate-700 transition hover:bg-slate-50 disabled:opacity-50",
        className,
      )}
      title={title}
      type="button"
      {...props}
    />
  );
}

function SmallAction({
  className,
  children,
  title,
  "aria-label": ariaLabel,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const accessibleLabel = ariaLabel ?? (typeof title === "string" ? title : undefined);
  return (
    <button
      aria-label={accessibleLabel}
      className={cn(
        "small-action inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-white hover:bg-slate-50 disabled:pointer-events-none disabled:opacity-50",
        className,
      )}
      title={title}
      type="button"
      {...props}
    >
      {children}
    </button>
  );
}

function VSCodeIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" className={className}>
      <path
        fill="#007ACC"
        d="M20.8 3.8 16.7 2 8.8 9.3 4.5 6 2.8 6.9v10.2l1.7.9 4.3-3.3 7.9 7.3 4.1-1.8V3.8Z"
      />
      <path fill="#1F9CF0" d="m16.7 7.5-5 4.5 5 4.5V7.5Z" />
      <path fill="#FFFFFF" fillOpacity="0.18" d="M16.7 2v20l4.1-1.8V3.8L16.7 2Z" />
    </svg>
  );
}

function Modal({
  title,
  children,
  onClose,
}: {
  title: string;
  children: React.ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="modal-backdrop fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-950/35 p-3 sm:items-center sm:p-4">
      <div className="modal-surface flex max-h-[calc(100dvh-1.5rem)] w-full max-w-xl flex-col overflow-hidden rounded-lg border border-border bg-white shadow-xl sm:max-h-[calc(100dvh-2rem)]">
        <div className="panel-header flex h-14 shrink-0 items-center justify-between gap-3 border-b border-border px-5">
          <h2 className="min-w-0 truncate text-base font-semibold">{title}</h2>
          <IconButton title="Close this dialog and return to the current Skill Manager view." onClick={onClose}>
            <X aria-hidden="true" className="h-4 w-4" />
          </IconButton>
        </div>
        <div className="min-h-0 overflow-y-auto p-5">{children}</div>
      </div>
    </div>
  );
}

function action(event: React.MouseEvent, callback: () => void) {
  event.preventDefault();
  event.stopPropagation();
  callback();
}

function basename(path: string) {
  return path.split(/[\\/]/).filter(Boolean).pop() || path;
}

function repositoryItemId(item: RepositoryPanelItem) {
  return item.sourceKey || item.repoId || item.id;
}

function repositoryItemTitle(item: RepositoryPanelItem) {
  if (item.alias) return item.alias;
  if (item.repoId) return basename(item.repoId);
  return basename(item.path);
}

function defaultRepoFolderName(repoId: string) {
  const parts = repoId
    .replace(/\.git$/, "")
    .split("/")
    .map((part) => part.trim())
    .filter(Boolean);
  if (parts.length >= 3) {
    return `${parts[parts.length - 2]}-${parts[parts.length - 1]}`;
  }
  return parts[parts.length - 1] || "repo";
}

function cloneFolderNameError(folderName: string) {
  if (!folderName) return "Folder name is required.";
  if (folderName === "." || folderName === "..") return "Choose a folder name other than '.' or '..'.";
  if (folderName.includes("/") || folderName.includes("\\")) return "Folder name cannot contain path separators.";
  if (folderName.startsWith("~") || /^[A-Za-z]:/.test(folderName)) return "Folder name must be relative to the selected parent.";
  return "";
}

function targetDirsLabel(targetDirs?: string[]) {
  if (!targetDirs?.length) {
    return "-";
  }
  if (targetDirs.length === 1) {
    return targetDirs[0];
  }
  return `${targetDirs.length} targets`;
}

function tagToneStyle(tag: string): CSSProperties {
  return TAG_TONES[hashString(tag) % TAG_TONES.length];
}

function hashString(value: string) {
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  return hash;
}

function keyboardResizeDelta(event: React.KeyboardEvent) {
  const step = event.shiftKey ? 24 : 8;
  if (event.key === "ArrowLeft") return -step;
  if (event.key === "ArrowRight") return step;
  return 0;
}

function adjustSkillColumnWidths(
  widths: SkillColumnWidths,
  leftKey: SkillColumnKey,
  rightKey: SkillColumnKey,
  delta: number,
) {
  const minDelta = MIN_SKILL_COLUMN_WIDTHS[leftKey] - widths[leftKey];
  const maxDelta = widths[rightKey] - MIN_SKILL_COLUMN_WIDTHS[rightKey];
  const adjustedDelta = clamp(delta, minDelta, maxDelta);
  return {
    ...widths,
    [leftKey]: widths[leftKey] + adjustedDelta,
    [rightKey]: widths[rightKey] - adjustedDelta,
  };
}

function useWorkbenchLayout() {
  const [layout, setLayout] = useState<WorkbenchLayout>(() => workbenchLayoutForWidth(window.innerWidth));

  useEffect(() => {
    function updateLayout() {
      const nextLayout = workbenchLayoutForWidth(window.innerWidth);
      setLayout((current) => (current === nextLayout ? current : nextLayout));
    }
    window.addEventListener("resize", updateLayout);
    return () => window.removeEventListener("resize", updateLayout);
  }, []);

  return layout;
}

function workbenchLayoutForWidth(width: number): WorkbenchLayout {
  if (width >= 1200) return "desktop";
  if (width >= 850) return "split";
  return "compact";
}

function buildWorkbenchGridColumns(layout: WorkbenchLayout, sourceWidth: number, detailWidth: number) {
  if (layout === "compact") {
    return "minmax(0, 1fr)";
  }
  if (layout === "split") {
    const detailColumn = `clamp(320px, min(${detailWidth}px, 44vw), ${MAX_DETAIL_WIDTH}px)`;
    return `minmax(0, 1fr) ${RESIZE_HANDLE_WIDTH}px ${detailColumn}`;
  }
  const fixedChromeWidth = RESIZE_HANDLE_WIDTH * 2;
  const sourceMin = `min(${MIN_SOURCE_WIDTH}px, calc((100% - ${fixedChromeWidth}px) * 0.24))`;
  const detailMin = `min(${MIN_DETAIL_WIDTH}px, calc((100% - ${fixedChromeWidth}px) * 0.34))`;
  const sourceMax = `min(${sourceWidth}px, calc((100% - ${fixedChromeWidth}px) * 0.28))`;
  const detailMax = `min(${detailWidth}px, calc((100% - ${fixedChromeWidth}px) * 0.45))`;
  const sourceColumn = `clamp(${sourceMin}, ${sourceMax}, ${MAX_SOURCE_WIDTH}px)`;
  const detailColumn = `clamp(${detailMin}, ${detailMax}, ${MAX_DETAIL_WIDTH}px)`;
  return `${sourceColumn} ${RESIZE_HANDLE_WIDTH}px minmax(0, 1fr) ${RESIZE_HANDLE_WIDTH}px ${detailColumn}`;
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function readStoredWidth(key: string, fallback: number, min: number, max: number) {
  const value = Number(localStorage.getItem(key));
  if (!Number.isFinite(value)) return fallback;
  return clamp(value, min, max);
}

function readStoredSkillColumnWidths() {
  try {
    const parsed = JSON.parse(localStorage.getItem(SKILLS_COLUMNS_KEY) ?? "");
    return normalizeSkillColumnWidths(parsed);
  } catch {
    return DEFAULT_SKILL_COLUMN_WIDTHS;
  }
}

function normalizeSkillColumnWidths(value: unknown): SkillColumnWidths {
  if (!value || typeof value !== "object") {
    return DEFAULT_SKILL_COLUMN_WIDTHS;
  }
  const widths = { ...DEFAULT_SKILL_COLUMN_WIDTHS };
  for (const key of skillColumnKeys) {
    const next = Number((value as Partial<SkillColumnWidths>)[key]);
    if (Number.isFinite(next)) {
      widths[key] = Math.max(MIN_SKILL_COLUMN_WIDTHS[key], next);
    }
  }
  const normalized = { ...DEFAULT_SKILL_COLUMN_WIDTHS };
  let remainingKeys = [...skillColumnKeys] as SkillColumnKey[];
  let remainingPercent = 100;
  while (remainingKeys.length > 0) {
    const totalWeight = remainingKeys.reduce((sum, key) => sum + widths[key], 0);
    if (totalWeight <= 0) return DEFAULT_SKILL_COLUMN_WIDTHS;
    const belowMinimum = remainingKeys.filter(
      (key) => (widths[key] / totalWeight) * remainingPercent < MIN_SKILL_COLUMN_WIDTHS[key],
    );
    if (belowMinimum.length === 0) {
      for (const key of remainingKeys) {
        normalized[key] = (widths[key] / totalWeight) * remainingPercent;
      }
      break;
    }
    for (const key of belowMinimum) {
      normalized[key] = MIN_SKILL_COLUMN_WIDTHS[key];
      remainingPercent -= MIN_SKILL_COLUMN_WIDTHS[key];
    }
    remainingKeys = remainingKeys.filter((key) => !belowMinimum.includes(key));
  }
  return normalized;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.round(value)));
}

export default App;
