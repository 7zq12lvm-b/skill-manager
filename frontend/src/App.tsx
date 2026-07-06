import { useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  Check,
  ChevronRight,
  Circle,
  CloudDownload,
  ExternalLink,
  Folder,
  FolderPlus,
  Loader2,
  RefreshCcw,
  Search,
  Settings,
  SlidersHorizontal,
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
  synced: "Synced",
  disabled: "Disabled",
  conflict: "Conflict",
  invalid: "Invalid",
  missing: "Missing",
  "missing-source": "Missing Source",
  "missing-path": "Missing Path",
  "needs-apply": "Needs Apply",
  "local-only": "Local Only",
  syncing: "Syncing",
  error: "Error",
};

const statusClass: Record<string, string> = {
  synced: "status-pill--synced",
  disabled: "status-pill--disabled",
  conflict: "status-pill--conflict",
  invalid: "status-pill--invalid",
  error: "status-pill--invalid",
  missing: "status-pill--missing",
  "missing-source": "status-pill--missing",
  "missing-path": "status-pill--missing",
  "needs-apply": "status-pill--syncing",
  "local-only": "status-pill--local-only",
  syncing: "status-pill--syncing",
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
const skillColumnKeys = ["enabled", "skill", "source", "status", "updated"] as const;
type SkillColumnKey = (typeof skillColumnKeys)[number];
type SkillColumnWidths = Record<SkillColumnKey, number>;
type RepositoryPanelItem = skillmgr.Repository | skillmgr.SkillSource;
const DEFAULT_SKILL_COLUMN_WIDTHS: SkillColumnWidths = {
  enabled: 14,
  skill: 38,
  source: 16,
  status: 16,
  updated: 16,
};
const MIN_SKILL_COLUMN_WIDTHS: SkillColumnWidths = {
  enabled: 13,
  skill: 24,
  source: 10,
  status: 12,
  updated: 12,
};

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
    enableSkillLocalOnly,
    disableSkill,
    removeSkillFromSync,
    applySync,
    adoptCurrentEnabledSkills,
    cloneRepository,
    resolveConflict,
    saveConfig,
    saveLLMConfig,
    generateSkillProfile,
    readSkillEnv,
    saveSkillEnv,
    saveSkillTags,
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
  const [skillToClone, setSkillToClone] = useState<skillmgr.Skill>();
  const [sourcePath, setSourcePath] = useState("");
  const [sourcePanelWidth, setSourcePanelWidth] = useState(() =>
    readStoredWidth(SOURCE_WIDTH_KEY, DEFAULT_SOURCE_WIDTH, MIN_SOURCE_WIDTH, MAX_SOURCE_WIDTH),
  );
  const [detailPanelWidth, setDetailPanelWidth] = useState(() =>
    readStoredWidth(DETAIL_WIDTH_KEY, DEFAULT_DETAIL_WIDTH, MIN_DETAIL_WIDTH, MAX_DETAIL_WIDTH),
  );
  const [skillColumnWidths, setSkillColumnWidths] = useState(readStoredSkillColumnWidths);
  const [generatingProfileIds, setGeneratingProfileIds] = useState<Set<string>>(() => new Set());
  const skillsTableRef = useRef<HTMLDivElement>(null);
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

  const filteredSkills = useMemo(() => {
    const skills = inventory?.skills ?? [];
    const normalizedQuery = query.trim().toLowerCase();
    return skills.filter((skill) => {
      const matchesSource = selectedSourceId === "all" || skill.sourceId === selectedSourceId || skill.repoId === selectedSourceId;
      const matchesStatus = statusFilter === "all" || skill.status === statusFilter;
      const matchesQuery =
        normalizedQuery.length === 0 ||
        skill.name.toLowerCase().includes(normalizedQuery) ||
        (skill.displayName ?? "").toLowerCase().includes(normalizedQuery) ||
        (skill.repoSubpath ?? "").toLowerCase().includes(normalizedQuery) ||
        skill.sourcePath.toLowerCase().includes(normalizedQuery) ||
        (skill.tags ?? []).some((tag) => tag.toLowerCase().includes(normalizedQuery));
      return matchesSource && matchesStatus && matchesQuery;
    });
  }, [inventory?.skills, query, selectedSourceId, statusFilter]);

  const selectedSkill =
    filteredSkills.find((skill) => skill.id === selectedSkillId) ??
    inventory?.skills?.find((skill) => skill.id === selectedSkillId) ??
    filteredSkills[0];
  const workbenchGridColumns = buildWorkbenchGridColumns(sourcePanelWidth, detailPanelWidth);

  function maybeGenerateProfileForSkill(skill: skillmgr.Skill) {
    if (
      !inventory?.syncConfigured ||
      !llmConfigReady(inventory.llmConfig) ||
      !skill.syncId ||
      !skill.sourcePath ||
      !skill.canSync ||
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

  return (
    <div className="app-shell flex h-screen min-w-0 flex-col overflow-hidden bg-background">
      <header className="app-topbar flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-5">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <div className="brand-lockup">
            <img className="brand-mark" src={logoUniversal} alt="" width={36} height={36} />
            <div className="min-w-0">
              <h1 className="brand-title">AI Agent Skill Manager</h1>
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
          {inventory?.syncConfigured ? (
            <>
              <Button variant="outline" onClick={adoptCurrentEnabledSkills} disabled={loading}>
                <Check aria-hidden="true" className="h-4 w-4" />
                Adopt
              </Button>
              <Button variant="outline" onClick={applySync} disabled={loading}>
                <CloudDownload aria-hidden="true" className={cn("h-4 w-4", loading && "animate-pulse")} />
                Apply Sync
              </Button>
            </>
          ) : (
            <Button variant="outline" onClick={() => setSettingsOpen(true)}>
              <CloudDownload aria-hidden="true" className="h-4 w-4" />
              Set Up Sync
            </Button>
          )}
          <IconButton title="Open primary target folder" onClick={() => inventory && openPath(primaryTargetDir(inventory.config))}>
            <Folder aria-hidden="true" className="h-4 w-4" />
          </IconButton>
          <Button variant="outline" onClick={rescan} disabled={loading}>
            <RefreshCcw aria-hidden="true" className={cn("h-4 w-4", loading && "animate-spin")} />
            Rescan All
          </Button>
          <IconButton title="Settings" onClick={() => setSettingsOpen(true)}>
            <Settings aria-hidden="true" className="h-4 w-4" />
          </IconButton>
        </div>
      </header>

      {error && (
        <div className="flex items-center justify-between border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-700">
          <span>{error}</span>
          <button className="rounded p-1 hover:bg-rose-100" onClick={clearError} title="Dismiss" aria-label="Dismiss error">
            <X aria-hidden="true" className="h-4 w-4" />
          </button>
        </div>
      )}

      {loading && <LoadingOverlay label={loadingLabel || "Working..."} />}

      <main
        className="workbench-grid grid min-h-0 flex-1 overflow-hidden"
        style={{
          gridTemplateColumns: workbenchGridColumns,
        }}
      >
        <aside className="workbench-panel workbench-panel--sources flex min-h-0 min-w-0 flex-col overflow-hidden bg-white">
          <PanelHeader title="Repositories">
            <IconButton title="Add repository or local folder" onClick={() => setAddSourceOpen(true)}>
              <FolderPlus aria-hidden="true" className="h-4 w-4" />
            </IconButton>
          </PanelHeader>
          <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3">
            {(inventory?.repositories ?? []).map((source) => (
              <div
                key={repositoryItemId(source)}
                role="button"
                tabIndex={0}
                className={cn(
                  "source-card w-full cursor-pointer rounded-md border p-3 text-left transition hover:bg-slate-50",
                  selectedSourceId === repositoryItemId(source) && "source-card--selected border-blue-300 bg-blue-50",
                )}
                onClick={() => setSelectedSourceId(repositoryItemId(source))}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    setSelectedSourceId(repositoryItemId(source));
                  }
                }}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{repositoryItemTitle(source)}</div>
                    <div className="truncate text-xs text-muted-foreground">{source.path}</div>
                    {"currentRef" in source && source.currentRef && (
                      <div className="truncate text-xs text-muted-foreground">{source.currentRef}</div>
                    )}
                  </div>
                  {source.errorCount > 0 || ("dirty" in source && source.dirty) ? (
                    <AlertTriangle aria-hidden="true" className="h-4 w-4 shrink-0 text-amber-600" />
                  ) : (
                    <Circle aria-hidden="true" className="h-4 w-4 shrink-0 text-emerald-600" />
                  )}
                </div>
                <div className="mt-3 flex items-center justify-between text-xs text-muted-foreground">
                  <span>{source.skillCount} skills</span>
                  <span>{formatDate(source.lastScannedAt)}</span>
                </div>
                <div className="mt-3 flex gap-1">
                  <SmallAction title="Open" onClick={(event) => action(event, () => openPath(source.path))}>
                    <ExternalLink aria-hidden="true" className="h-3.5 w-3.5" />
                  </SmallAction>
                  <SmallAction title="Alias" onClick={(event) => action(event, () => setSourceToEdit(source))}>
                    <SlidersHorizontal aria-hidden="true" className="h-3.5 w-3.5" />
                  </SmallAction>
                  <SmallAction
                    title={("dirty" in source && source.dirty) ? "Repository has uncommitted changes" : "Pull latest"}
                    disabled={loading || ("dirty" in source && source.dirty)}
                    onClick={(event) => action(event, () => pullRepository(repositoryItemId(source)))}
                  >
                    <CloudDownload aria-hidden="true" className={cn("h-3.5 w-3.5", loading && "animate-pulse")} />
                  </SmallAction>
                  <SmallAction title="Remove" onClick={(event) => action(event, () => setSourceToRemove(source))}>
                    <Trash2 aria-hidden="true" className="h-3.5 w-3.5" />
                  </SmallAction>
                </div>
                {pullResults[repositoryItemId(source)] && (
                  <div className="mt-2 truncate rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-xs text-emerald-700">
                    {pullResults[repositoryItemId(source)]}
                  </div>
                )}
              </div>
            ))}
            {(inventory?.sources ?? []).length > 0 && (
              <div className="pt-2 text-xs font-semibold uppercase tracking-normal text-muted-foreground">Local Folders</div>
            )}
            {(inventory?.sources ?? []).map((source) => (
              <div
                key={source.id}
                role="button"
                tabIndex={0}
                className={cn(
                  "source-card w-full cursor-pointer rounded-md border p-3 text-left transition hover:bg-slate-50",
                  selectedSourceId === source.id && "source-card--selected border-blue-300 bg-blue-50",
                )}
                onClick={() => setSelectedSourceId(source.id)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    setSelectedSourceId(source.id);
                  }
                }}
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">{source.alias || basename(source.path)}</div>
                    <div className="truncate text-xs text-muted-foreground">{source.path}</div>
                  </div>
                  {source.errorCount > 0 ? (
                    <AlertTriangle aria-hidden="true" className="h-4 w-4 shrink-0 text-amber-600" />
                  ) : (
                    <Circle aria-hidden="true" className="h-4 w-4 shrink-0 text-emerald-600" />
                  )}
                </div>
                <div className="mt-3 flex items-center justify-between text-xs text-muted-foreground">
                  <span>{source.skillCount} skills</span>
                  <span>{formatDate(source.lastScannedAt)}</span>
                </div>
                <div className="mt-3 flex gap-1">
                  <SmallAction title="Open" onClick={(event) => action(event, () => openPath(source.path))}>
                    <ExternalLink aria-hidden="true" className="h-3.5 w-3.5" />
                  </SmallAction>
                  <SmallAction title="Alias" onClick={(event) => action(event, () => setSourceToEdit(source))}>
                    <SlidersHorizontal aria-hidden="true" className="h-3.5 w-3.5" />
                  </SmallAction>
                  <SmallAction title="Remove" onClick={(event) => action(event, () => setSourceToRemove(source))}>
                    <Trash2 aria-hidden="true" className="h-3.5 w-3.5" />
                  </SmallAction>
                </div>
              </div>
            ))}
            <button
              className={cn(
                "source-card source-card--all w-full rounded-md border p-3 text-left text-sm",
                selectedSourceId === "all" && "source-card--selected border-blue-300 bg-blue-50",
              )}
              onClick={() => setSelectedSourceId("all")}
            >
              All Sources
            </button>
          </div>
        </aside>

        <ResizeHandle
          label="Resize Repositories"
          onKeyDown={(event) => resizePanelByKeyboard("source", event)}
          onPointerDown={(event) => startColumnResize("source", event)}
        />

        <section className="workbench-panel workbench-panel--skills flex min-h-0 min-w-0 flex-col overflow-hidden bg-slate-50">
          <PanelHeader title="Skills">
            <Button variant="outline" onClick={rescan} disabled={loading}>
              <RefreshCcw aria-hidden="true" className={cn("h-4 w-4", loading && "animate-spin")} />
              Rescan
            </Button>
          </PanelHeader>
          <div className="filter-bar shrink-0 flex flex-wrap gap-2 border-b border-border bg-white p-3">
            <div className="relative min-w-0 flex-1">
              <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <input
                aria-label="Search skills"
                autoComplete="off"
                name="skill-search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search skills…"
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
          <div ref={skillsTableRef} className="min-h-0 flex-1 overflow-auto">
            <table className="w-full table-fixed border-collapse text-sm">
              <colgroup>
                {skillColumnKeys.map((key) => (
                  <col key={key} style={{ width: `${skillColumnWidths[key]}%` }} />
                ))}
              </colgroup>
              <thead className="text-left text-xs font-medium text-muted-foreground">
                <tr className="border-b border-border">
                  <SkillHeaderCell
                    label="On"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("enabled", "skill", event)}
                    onResize={(event) => startSkillColumnResize("enabled", "skill", event)}
                  />
                  <SkillHeaderCell
                    label="Skill"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("skill", "source", event)}
                    onResize={(event) => startSkillColumnResize("skill", "source", event)}
                  />
                  <SkillHeaderCell
                    label="Source"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("source", "status", event)}
                    onResize={(event) => startSkillColumnResize("source", "status", event)}
                  />
                  <SkillHeaderCell
                    label="Status"
                    onKeyResize={(event) => resizeSkillColumnByKeyboard("status", "updated", event)}
                    onResize={(event) => startSkillColumnResize("status", "updated", event)}
                  />
                  <SkillHeaderCell label="Updated" />
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
                  >
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
                      <TagList tags={skill.tags} compact />
                    </td>
                    <td className="min-w-0 overflow-hidden px-3 py-2 text-muted-foreground">
                      <div className="truncate">{skill.sourceAlias || skill.repoId || skill.sourceId}</div>
                      {skill.repoSubpath && <div className="truncate text-xs">{skill.repoSubpath}</div>}
                    </td>
                    <td className="min-w-0 overflow-hidden px-3 py-2">
                      <StatusPill status={skill.status} />
                    </td>
                    <td className="min-w-0 overflow-hidden px-3 py-2 text-xs text-muted-foreground">
                      <div className="truncate">{formatDate(skill.updatedAt)}</div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {filteredSkills.length === 0 && (
              <div className="empty-state p-8 text-center text-sm text-muted-foreground">
                No skills match these filters. Clear search or choose another status.
              </div>
            )}
          </div>
        </section>

        <ResizeHandle
          label="Resize Skill Detail"
          onKeyDown={(event) => resizePanelByKeyboard("detail", event)}
          onPointerDown={(event) => startColumnResize("detail", event)}
        />

        <aside className="workbench-panel workbench-panel--detail flex min-h-0 min-w-0 flex-col overflow-hidden bg-white">
          <PanelHeader title="Skill Detail">
            <IconButton
              title="Open skill folder in VS Code"
              disabled={!selectedSkill}
              onClick={() => selectedSkill && openInVSCode(selectedSkill.sourcePath)}
            >
              <VSCodeIcon className="h-4 w-4" />
            </IconButton>
          </PanelHeader>
          <SkillDetail
            skill={selectedSkill}
            syncConfigured={Boolean(inventory?.syncConfigured)}
            llmConfig={inventory?.llmConfig}
            isGeneratingProfile={Boolean(selectedSkill && generatingProfileIds.has(selectedSkill.id))}
            onEnable={enableSkill}
            onEnableLocalOnly={enableSkillLocalOnly}
            onResolve={resolveConflict}
            onGenerateProfile={generateProfile}
            onReadEnv={readSkillEnv}
            onSaveEnv={saveSkillEnv}
            onSaveTags={saveSkillTags}
            onRemoveFromSync={removeSkillFromSync}
            onInstallRepository={setSkillToClone}
          />
        </aside>
      </main>

      {addSourceOpen && (
        <Modal title="Add Repository" onClose={() => setAddSourceOpen(false)}>
          <div className="space-y-4">
            <label className="block text-sm font-medium">
              Repository or Local Folder
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
              Git folders are tracked as repositories and scanned recursively for SKILL.md. Non-git folders stay local-only.
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setAddSourceOpen(false)}>
                Cancel
              </Button>
              <Button onClick={submitSource}>Add</Button>
            </div>
          </div>
        </Modal>
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
            await renameSource(repositoryItemId(sourceToEdit), alias);
            setSourceToEdit(undefined);
          }}
        />
      )}

      {sourceToRemove && (
        <RemoveSourceModal
          source={sourceToRemove}
          onClose={() => setSourceToRemove(undefined)}
          onRemove={async () => {
            await removeSource(repositoryItemId(sourceToRemove));
            setSourceToRemove(undefined);
          }}
        />
      )}

      {skillToClone && (
        <CloneRepositoryModal
          skill={skillToClone}
          onBrowseParent={browseForRepositoryFolder}
          onClose={() => setSkillToClone(undefined)}
          onClone={async (repoId, cloneUrl, parentDir, folderName) => {
            await cloneRepository(repoId, cloneUrl, parentDir, folderName);
            setSkillToClone(undefined);
          }}
        />
      )}
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
      title={label}
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
            title="Resize column"
            onKeyDown={onKeyResize}
            onPointerDown={onResize}
            className="column-resize absolute -right-3 top-1/2 h-6 w-2 -translate-y-1/2 cursor-col-resize rounded hover:bg-blue-400/50"
          />
        )}
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
  const disabled = ["invalid", "error", "missing", "missing-source", "missing-path"].includes(skill.status);
  return (
    <button
      aria-label={`${checked ? "Disable" : "Enable"} ${skill.name}`}
      aria-checked={checked}
      role="switch"
      title={checked ? "Disable" : "Enable"}
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
  return skill.isActive || skill.status === "synced" || skill.status === "syncing" || skill.status === "local-only";
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

function SkillDetail({
  skill,
  syncConfigured,
  llmConfig,
  isGeneratingProfile,
  onEnable,
  onEnableLocalOnly,
  onResolve,
  onGenerateProfile,
  onReadEnv,
  onSaveEnv,
  onSaveTags,
  onRemoveFromSync,
  onInstallRepository,
}: {
  skill?: skillmgr.Skill;
  syncConfigured: boolean;
  llmConfig?: skillmgr.SyncLLMConfig;
  isGeneratingProfile: boolean;
  onEnable: (skillId: string) => Promise<void>;
  onEnableLocalOnly: (skillId: string) => Promise<void>;
  onResolve: (skillId: string) => Promise<void>;
  onGenerateProfile: (skillId: string, force?: boolean) => Promise<void>;
  onReadEnv: (skillId: string) => Promise<string>;
  onSaveEnv: (skillId: string, content: string) => Promise<void>;
  onSaveTags: (skillId: string, tags: string[]) => Promise<void>;
  onRemoveFromSync: (skillId: string) => Promise<void>;
  onInstallRepository: (skill: skillmgr.Skill) => void;
}) {
  if (!skill) {
    return <div className="p-5 text-sm text-muted-foreground">No skill selected.</div>;
  }
  const activeLinkedPaths = linkedSkillPaths(skill);
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

      <SyncSection
        skill={skill}
        syncConfigured={syncConfigured}
        onEnable={onEnable}
        onEnableLocalOnly={onEnableLocalOnly}
        onRemoveFromSync={onRemoveFromSync}
        onInstallRepository={onInstallRepository}
      />

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
                >
                  Replace Existing Target
                </Button>
              </div>
            )}
          </div>
        </DetailSection>
      )}

      <DetailSection title="Files">
        <div className="flex min-w-0 flex-wrap gap-2">
          {(skill.files ?? []).map((file) => (
            <span key={file} className="file-chip max-w-full break-all rounded-md border border-border bg-slate-50 px-2 py-1 text-xs">
              {file}
            </span>
          ))}
        </div>
      </DetailSection>

      {skill.preview && (
        <DetailSection title={`Preview: ${skill.previewFile}`}>
          <pre className="code-preview code-preview--wrap max-h-72 max-w-full overflow-auto rounded-md border border-border bg-slate-950 p-3 text-xs leading-5 text-slate-100">
            {skill.preview}
          </pre>
        </DetailSection>
      )}

      <EnvEditor skill={skill} onReadEnv={onReadEnv} onSaveEnv={onSaveEnv} />
    </div>
  );
}

function SyncSection({
  skill,
  syncConfigured,
  onEnable,
  onEnableLocalOnly,
  onRemoveFromSync,
  onInstallRepository,
}: {
  skill: skillmgr.Skill;
  syncConfigured: boolean;
  onEnable: (skillId: string) => Promise<void>;
  onEnableLocalOnly: (skillId: string) => Promise<void>;
  onRemoveFromSync: (skillId: string) => Promise<void>;
  onInstallRepository: (skill: skillmgr.Skill) => void;
}) {
  const desired =
    skill.desiredEnabled === undefined ? "Not in sync" : skill.desiredEnabled ? "Enabled in sync" : "Disabled in sync";
  return (
    <DetailSection title="Sync">
      <div className="min-w-0 space-y-2 rounded-md border border-border bg-slate-50 p-3 text-xs">
        <div className="grid min-w-0 grid-cols-[minmax(0,96px)_minmax(0,1fr)] gap-2">
          <span className="text-muted-foreground">desired</span>
          <span className="truncate">{desired}</span>
        </div>
        <div className="grid min-w-0 grid-cols-[minmax(0,96px)_minmax(0,1fr)] gap-2">
          <span className="text-muted-foreground">applied</span>
          <span className="truncate">{skill.isActive ? "Enabled here" : "Not enabled here"}</span>
        </div>
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
        <div className="flex flex-wrap gap-2 pt-1">
          {syncConfigured && !skill.isSynced && skill.canSync && skill.isActive && (
            <Button variant="outline" onClick={() => void onEnable(skill.id)}>
              Add to Sync
            </Button>
          )}
          {syncConfigured && skill.canSync && !skill.isActive && (
            <Button variant="outline" onClick={() => void onEnableLocalOnly(skill.id)}>
              Enable Local Only
            </Button>
          )}
          {skill.isSynced && (
            <Button variant="outline" onClick={() => void onRemoveFromSync(skill.id)}>
              Remove From Sync
            </Button>
          )}
          {skill.status === "missing-source" && skill.repoId && (
            <Button variant="outline" onClick={() => onInstallRepository(skill)}>
              Install Repository
            </Button>
          )}
        </div>
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
  const canGenerate = Boolean(syncConfigured && skill.canSync && skill.syncId && skill.sourcePath && llmConfigReady(llmConfig));
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
              title={canGenerate ? undefined : "Configure sync and LLM first"}
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
        <Button variant="outline" onClick={() => void addDraftTag()} disabled={saving || !draft.trim()}>
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
}: {
  tags?: string[];
  compact?: boolean;
  disabled?: boolean;
  onRemove?: (tag: string) => void;
}) {
  const visibleTags = tags ?? [];
  if (visibleTags.length === 0) {
    if (compact) return null;
    return <div className="text-xs text-muted-foreground">No tags yet.</div>;
  }
  return (
    <div className={cn("tag-list flex min-w-0 flex-wrap gap-1.5", compact && "mt-1")}>
      {visibleTags.map((tag) => (
        <span key={tag} className="tag-chip inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-0.5 text-xs">
          <span className="truncate">{tag}</span>
          {onRemove && (
            <button
              aria-label={`Remove ${tag} tag`}
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
    </div>
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
            <Button variant="outline" onClick={reload} disabled={loading || saving}>
              Reload
            </Button>
            <Button onClick={save} disabled={loading || saving || !dirty}>
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
                <IconButton title="Remove target directory" onClick={() => removeTargetDir(index)}>
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
              <Button variant="outline" onClick={() => void addTargetDir()}>
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
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={() => onSave(config, llmConfig)}>Save</Button>
        </div>
      </div>
    </Modal>
  );
}

function CloneRepositoryModal({
  skill,
  onBrowseParent,
  onClose,
  onClone,
}: {
  skill: skillmgr.Skill;
  onBrowseParent: () => Promise<string>;
  onClose: () => void;
  onClone: (repoId: string, cloneUrl: string, parentDir: string, folderName: string) => Promise<void>;
}) {
  const repoId = skill.repoId ?? "";
  const [cloneUrl, setCloneUrl] = useState(skill.cloneUrl || defaultCloneUrl(repoId));
  const [parentDir, setParentDir] = useState("");
  const [folderName, setFolderName] = useState(defaultRepoFolderName(repoId));
  const [cloning, setCloning] = useState(false);
  const finalPath = parentDir && folderName ? `${parentDir.replace(/[\\/]$/, "")}/${folderName}` : "";

  async function clone() {
    setCloning(true);
    try {
      await onClone(repoId, cloneUrl.trim(), parentDir.trim(), folderName.trim());
    } finally {
      setCloning(false);
    }
  }

  return (
    <Modal title="Install Repository" onClose={onClose}>
      <div className="space-y-4">
        <div className="rounded-md border border-border bg-slate-50 p-3 text-xs">
          <div className="break-all font-mono">{repoId}</div>
          {skill.repoSubpath && <div className="mt-1 break-all text-muted-foreground">{skill.repoSubpath}</div>}
        </div>
        <label className="block text-sm font-medium">
          Clone URL
          <input
            autoComplete="off"
            name="clone-url"
            value={cloneUrl}
            onChange={(event) => setCloneUrl(event.target.value)}
            className="mt-2 h-9 w-full rounded-md border border-input px-3 text-sm"
          />
        </label>
        <label className="block text-sm font-medium">
          Parent folder
          <div className="mt-2 flex gap-2">
            <input
              autoComplete="off"
              name="clone-parent"
              value={parentDir}
              onChange={(event) => setParentDir(event.target.value)}
              className="h-9 min-w-0 flex-1 rounded-md border border-input px-3 text-sm"
              placeholder="/Users/me/Code"
            />
            <Button
              variant="outline"
              onClick={async () => {
                const path = await onBrowseParent();
                if (path) setParentDir(path);
              }}
            >
              Browse
            </Button>
          </div>
        </label>
        <label className="block text-sm font-medium">
          Folder name
          <input
            autoComplete="off"
            name="clone-folder-name"
            value={folderName}
            onChange={(event) => setFolderName(event.target.value)}
            className="mt-2 h-9 w-full rounded-md border border-input px-3 text-sm"
          />
        </label>
        {finalPath && <div className="break-all rounded-md border border-border bg-slate-50 p-3 font-mono text-xs">{finalPath}</div>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={cloning}>
            Cancel
          </Button>
          <Button onClick={clone} disabled={cloning || !repoId || !cloneUrl.trim() || !parentDir.trim() || !folderName.trim()}>
            {cloning ? "Cloning…" : "Clone"}
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
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            Cancel
          </Button>
          <Button onClick={save} disabled={saving}>
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
          <Button variant="ghost" onClick={onClose} disabled={removing}>
            Cancel
          </Button>
          <Button onClick={remove} disabled={removing}>
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
      {status === "synced" && <Check aria-hidden="true" className="h-3 w-3" />}
      {status === "conflict" && <AlertTriangle aria-hidden="true" className="h-3 w-3" />}
      {(status === "missing-source" || status === "missing-path") && <AlertTriangle aria-hidden="true" className="h-3 w-3" />}
      {status === "syncing" && <Loader2 aria-hidden="true" className="h-3 w-3 animate-spin" />}
      {status === "needs-apply" && <Loader2 aria-hidden="true" className="h-3 w-3" />}
      {statusLabels[status] ?? status}
    </span>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="detail-section mb-5 min-w-0">
      <h3 className="mb-2 truncate text-xs font-semibold uppercase tracking-normal text-muted-foreground">{title}</h3>
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
    <div className="modal-backdrop fixed inset-0 z-50 flex items-center justify-center bg-slate-950/35 p-4">
      <div className="modal-surface w-full max-w-xl rounded-lg border border-border bg-white shadow-xl">
        <div className="panel-header flex h-14 items-center justify-between border-b border-border px-5">
          <h2 className="text-base font-semibold">{title}</h2>
          <IconButton title="Close" onClick={onClose}>
            <X aria-hidden="true" className="h-4 w-4" />
          </IconButton>
        </div>
        <div className="p-5">{children}</div>
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
  return "repoId" in item && item.repoId ? item.repoId : item.id;
}

function repositoryItemTitle(item: RepositoryPanelItem) {
  if (item.alias) return item.alias;
  if ("repoId" in item && item.repoId) return basename(item.repoId);
  return basename(item.path);
}

function defaultRepoFolderName(repoId: string) {
  return basename(repoId).replace(/\.git$/, "") || "repo";
}

function defaultCloneUrl(repoId: string) {
  return repoId ? `https://${repoId}.git` : "";
}

function primaryTargetDir(config: skillmgr.Config) {
  return config.targetDirs?.[0] ?? "~/.agents/skills";
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

function buildWorkbenchGridColumns(sourceWidth: number, detailWidth: number) {
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
  const total = skillColumnKeys.reduce((sum, key) => sum + widths[key], 0);
  if (total <= 0) {
    return DEFAULT_SKILL_COLUMN_WIDTHS;
  }
  const normalized = { ...widths };
  for (const key of skillColumnKeys) {
    normalized[key] = (widths[key] / total) * 100;
  }
  return normalized;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.round(value)));
}

export default App;
