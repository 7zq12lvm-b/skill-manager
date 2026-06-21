import { useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  Check,
  ChevronRight,
  Circle,
  ExternalLink,
  Folder,
  FolderPlus,
  Loader2,
  RefreshCcw,
  Search,
  Settings,
  SlidersHorizontal,
  Trash2,
  X,
} from "lucide-react";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { skillmgr } from "../wailsjs/go/models";
import { cn } from "./lib/utils";
import { useSkillStore } from "./store/useSkillStore";
import "./App.css";

const statusLabels: Record<string, string> = {
  synced: "Synced",
  disabled: "Disabled",
  conflict: "Conflict",
  invalid: "Invalid",
  missing: "Missing",
  syncing: "Syncing",
  error: "Error",
};

const statusClass: Record<string, string> = {
  synced: "border-emerald-200 bg-emerald-50 text-emerald-700",
  disabled: "border-slate-200 bg-slate-50 text-slate-600",
  conflict: "border-amber-200 bg-amber-50 text-amber-700",
  invalid: "border-rose-200 bg-rose-50 text-rose-700",
  error: "border-rose-200 bg-rose-50 text-rose-700",
  missing: "border-slate-200 bg-slate-50 text-slate-600",
  syncing: "border-blue-200 bg-blue-50 text-blue-700",
};

const SOURCE_WIDTH_KEY = "skill-manager:source-panel-width";
const DETAIL_WIDTH_KEY = "skill-manager:detail-panel-width";
const DEFAULT_SOURCE_WIDTH = 260;
const DEFAULT_DETAIL_WIDTH = 340;
const MIN_SOURCE_WIDTH = 180;
const MAX_SOURCE_WIDTH = 420;
const MIN_DETAIL_WIDTH = 260;
const MAX_DETAIL_WIDTH = 560;

function App() {
  const {
    inventory,
    selectedSkillId,
    selectedSourceId,
    statusFilter,
    query,
    loading,
    error,
    setInventory,
    load,
    rescan,
    addSource,
    browseAndAddSource,
    removeSource,
    renameSource,
    enableSkill,
    disableSkill,
    resolveConflict,
    saveConfig,
    readSkillEnv,
    saveSkillEnv,
    openPath,
    selectSkill,
    setSelectedSourceId,
    setStatusFilter,
    setQuery,
    clearError,
  } = useSkillStore();
  const [addSourceOpen, setAddSourceOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [sourcePath, setSourcePath] = useState("");
  const [sourcePanelWidth, setSourcePanelWidth] = useState(() =>
    readStoredWidth(SOURCE_WIDTH_KEY, DEFAULT_SOURCE_WIDTH, MIN_SOURCE_WIDTH, MAX_SOURCE_WIDTH),
  );
  const [detailPanelWidth, setDetailPanelWidth] = useState(() =>
    readStoredWidth(DETAIL_WIDTH_KEY, DEFAULT_DETAIL_WIDTH, MIN_DETAIL_WIDTH, MAX_DETAIL_WIDTH),
  );

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

  const filteredSkills = useMemo(() => {
    const skills = inventory?.skills ?? [];
    const normalizedQuery = query.trim().toLowerCase();
    return skills.filter((skill) => {
      const matchesSource = selectedSourceId === "all" || skill.sourceId === selectedSourceId;
      const matchesStatus = statusFilter === "all" || skill.status === statusFilter;
      const matchesQuery =
        normalizedQuery.length === 0 ||
        skill.name.toLowerCase().includes(normalizedQuery) ||
        skill.sourcePath.toLowerCase().includes(normalizedQuery);
      return matchesSource && matchesStatus && matchesQuery;
    });
  }, [inventory?.skills, query, selectedSourceId, statusFilter]);

  const selectedSkill =
    filteredSkills.find((skill) => skill.id === selectedSkillId) ??
    inventory?.skills?.find((skill) => skill.id === selectedSkillId) ??
    filteredSkills[0];

  async function submitSource() {
    if (!sourcePath.trim()) return;
    await addSource(sourcePath.trim());
    setSourcePath("");
    setAddSourceOpen(false);
  }

  async function requestDisable(skill: skillmgr.Skill) {
    const ok = window.confirm(
      `Disable ${skill.name}?\n\nThis removes only the symlink in the target skill directory. The original skill folder stays in place.`,
    );
    if (ok) await disableSkill(skill.id);
  }

  async function requestRemoveSource(source: skillmgr.SkillSource) {
    const ok = window.confirm(
      `Remove ${source.alias || source.path} from scanning?\n\nSource files and existing symlinks will not be deleted.`,
    );
    if (ok) await removeSource(source.id);
  }

  async function requestRenameSource(source: skillmgr.SkillSource) {
    const alias = window.prompt("Source alias", source.alias || "");
    if (alias !== null) await renameSource(source.id, alias);
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

  return (
    <div className="flex h-screen min-w-0 flex-col overflow-hidden bg-background">
      <header className="flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border bg-white px-4 py-3 sm:px-5">
        <div className="flex min-w-0 flex-wrap items-center gap-3">
          <div className="min-w-0">
            <h1 className="text-lg font-semibold tracking-normal">AI Agent Skill Manager</h1>
            <p className="max-w-[calc(100vw-2rem)] truncate text-xs text-muted-foreground sm:max-w-[520px]">
              Target: {inventory?.config?.targetDir ?? "-"}
            </p>
          </div>
          {inventory && <SummaryBar summary={inventory.summary} />}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <IconButton title="Open target folder" onClick={() => inventory && openPath(inventory.config.targetDir)}>
            <Folder className="h-4 w-4" />
          </IconButton>
          <Button variant="outline" onClick={rescan} disabled={loading}>
            <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
            Rescan All
          </Button>
          <IconButton title="Settings" onClick={() => setSettingsOpen(true)}>
            <Settings className="h-4 w-4" />
          </IconButton>
        </div>
      </header>

      {error && (
        <div className="flex items-center justify-between border-b border-rose-200 bg-rose-50 px-5 py-2 text-sm text-rose-700">
          <span>{error}</span>
          <button className="rounded p-1 hover:bg-rose-100" onClick={clearError} title="Dismiss">
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      <main
        className="grid min-h-0 flex-1 overflow-auto"
        style={{
          gridTemplateColumns: `${sourcePanelWidth}px 8px minmax(420px, 1fr) 8px ${detailPanelWidth}px`,
        }}
      >
        <aside className="min-h-0 bg-white">
          <PanelHeader title="Skill Sources">
            <IconButton title="Add source" onClick={() => setAddSourceOpen(true)}>
              <FolderPlus className="h-4 w-4" />
            </IconButton>
          </PanelHeader>
          <div className="space-y-2 overflow-y-auto p-3">
            {(inventory?.sources ?? []).map((source) => (
              <div
                key={source.id}
                role="button"
                tabIndex={0}
                className={cn(
                  "w-full cursor-pointer rounded-md border p-3 text-left transition hover:bg-slate-50",
                  selectedSourceId === source.id ? "border-blue-300 bg-blue-50" : "border-border bg-white",
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
                    <AlertTriangle className="h-4 w-4 shrink-0 text-amber-600" />
                  ) : (
                    <Circle className="h-4 w-4 shrink-0 text-emerald-600" />
                  )}
                </div>
                <div className="mt-3 flex items-center justify-between text-xs text-muted-foreground">
                  <span>{source.skillCount} skills</span>
                  <span>{formatDate(source.lastScannedAt)}</span>
                </div>
                <div className="mt-3 flex gap-1">
                  <SmallAction title="Open" onClick={(event) => action(event, () => openPath(source.path))}>
                    <ExternalLink className="h-3.5 w-3.5" />
                  </SmallAction>
                  <SmallAction title="Alias" onClick={(event) => action(event, () => requestRenameSource(source))}>
                    <SlidersHorizontal className="h-3.5 w-3.5" />
                  </SmallAction>
                  <SmallAction title="Remove" onClick={(event) => action(event, () => requestRemoveSource(source))}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </SmallAction>
                </div>
              </div>
            ))}
            <button
              className={cn(
                "w-full rounded-md border p-3 text-left text-sm",
                selectedSourceId === "all" ? "border-blue-300 bg-blue-50" : "border-dashed border-border bg-white",
              )}
              onClick={() => setSelectedSourceId("all")}
            >
              All Sources
            </button>
          </div>
        </aside>

        <ResizeHandle label="Resize Skill Sources" onPointerDown={(event) => startColumnResize("source", event)} />

        <section className="min-h-0 bg-slate-50">
          <PanelHeader title="Skills">
            <Button variant="outline" onClick={rescan} disabled={loading}>
              <RefreshCcw className={cn("h-4 w-4", loading && "animate-spin")} />
              Rescan
            </Button>
          </PanelHeader>
          <div className="flex flex-wrap gap-2 border-b border-border bg-white p-3">
            <div className="relative min-w-0 flex-1">
              <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search skill..."
                className="h-9 w-full rounded-md border border-input bg-white pl-8 pr-3 text-sm"
              />
            </div>
            <select
              value={selectedSourceId}
              onChange={(event) => setSelectedSourceId(event.target.value)}
              className="h-9 min-w-[150px] flex-1 rounded-md border border-input bg-white px-2 text-sm sm:flex-none"
            >
              <option value="all">All Sources</option>
              {(inventory?.sources ?? []).map((source) => (
                <option key={source.id} value={source.id}>
                  {source.alias || basename(source.path)}
                </option>
              ))}
            </select>
            <select
              value={statusFilter}
              onChange={(event) => setStatusFilter(event.target.value)}
              className="h-9 min-w-[130px] flex-1 rounded-md border border-input bg-white px-2 text-sm sm:flex-none"
            >
              <option value="all">All Status</option>
              {Object.entries(statusLabels).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </div>
          <div className="min-h-0 overflow-auto">
            <table className="min-w-[640px] w-full border-collapse text-sm">
              <thead className="sticky top-0 bg-slate-100 text-left text-xs font-medium text-muted-foreground">
                <tr className="border-b border-border">
                  <th className="w-16 px-3 py-2">On</th>
                  <th className="px-3 py-2">Skill</th>
                  <th className="px-3 py-2">Source</th>
                  <th className="px-3 py-2">Status</th>
                  <th className="px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {filteredSkills.map((skill) => (
                  <tr
                    key={skill.id}
                    className={cn(
                      "cursor-pointer border-b border-border bg-white hover:bg-blue-50/50",
                      selectedSkill?.id === skill.id && "bg-blue-50",
                    )}
                    onClick={() => selectSkill(skill.id)}
                  >
                    <td className="px-3 py-2">
                      <SkillSwitch
                        skill={skill}
                        onEnable={() => enableSkill(skill.id)}
                        onDisable={() => disableSkill(skill.id)}
                      />
                    </td>
                    <td className="px-3 py-2">
                      <div className="font-medium">{skill.name}</div>
                      <div className="max-w-[260px] truncate text-xs text-muted-foreground">{skill.sourcePath}</div>
                    </td>
                    <td className="px-3 py-2 text-muted-foreground">{skill.sourceAlias || skill.sourceId}</td>
                    <td className="px-3 py-2">
                      <StatusPill status={skill.status} />
                    </td>
                    <td className="whitespace-nowrap px-3 py-2 text-xs text-muted-foreground">
                      {formatDate(skill.updatedAt)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {filteredSkills.length === 0 && (
              <div className="p-8 text-center text-sm text-muted-foreground">No skills match the current filters.</div>
            )}
          </div>
        </section>

        <ResizeHandle label="Resize Skill Detail" onPointerDown={(event) => startColumnResize("detail", event)} />

        <aside className="min-h-0 bg-white">
          <PanelHeader title="Skill Detail" />
          <SkillDetail
            skill={selectedSkill}
            onOpen={openPath}
            onEnable={enableSkill}
            onDisable={requestDisable}
            onResolve={resolveConflict}
            onReadEnv={readSkillEnv}
            onSaveEnv={saveSkillEnv}
          />
        </aside>
      </main>

      {addSourceOpen && (
        <Modal title="Add Skill Source" onClose={() => setAddSourceOpen(false)}>
          <div className="space-y-4">
            <label className="block text-sm font-medium">
              Source Directory
              <div className="mt-2 flex gap-2">
                <input
                  value={sourcePath}
                  onChange={(event) => setSourcePath(event.target.value)}
                  className="h-9 min-w-0 flex-1 rounded-md border border-input px-3 text-sm"
                  placeholder="/Users/yusuf/dev/skills"
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
              Each first-level subfolder is scanned as one skill.
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setAddSourceOpen(false)}>
                Cancel
              </Button>
              <Button onClick={submitSource}>Add Source</Button>
            </div>
          </div>
        </Modal>
      )}

      {settingsOpen && inventory && (
        <SettingsModal
          inventory={inventory}
          onClose={() => setSettingsOpen(false)}
          onSave={async (config) => {
            await saveConfig(config);
            setSettingsOpen(false);
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

function SummaryItem({ label, value, tone = "slate" }: { label: string; value: number; tone?: string }) {
  const tones: Record<string, string> = {
    slate: "bg-slate-100 text-slate-700",
    emerald: "bg-emerald-50 text-emerald-700",
    amber: "bg-amber-50 text-amber-700",
    rose: "bg-rose-50 text-rose-700",
  };
  return (
    <span className={cn("rounded-md px-2 py-1 font-medium", tones[tone])}>
      {value} {label}
    </span>
  );
}

function ResizeHandle({
  label,
  onPointerDown,
}: {
  label: string;
  onPointerDown: (event: React.PointerEvent) => void;
}) {
  return (
    <div
      aria-label={label}
      role="separator"
      tabIndex={0}
      title={label}
      onPointerDown={onPointerDown}
      className="group flex min-h-0 cursor-col-resize items-stretch justify-center bg-white"
    >
      <div className="w-px bg-border transition group-hover:w-1 group-hover:bg-blue-400" />
    </div>
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
  const disabled = ["invalid", "error"].includes(skill.status) || (skill.status === "conflict" && !checked);
  return (
    <button
      title={checked ? "Disable" : "Enable"}
      disabled={disabled}
      onClick={(event) => {
        event.stopPropagation();
        checked ? onDisable() : onEnable();
      }}
      className={cn(
        "relative h-5 w-9 rounded-full border transition disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "border-emerald-500 bg-emerald-500" : "border-slate-300 bg-slate-200",
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
  return skill.isActive || skill.status === "synced";
}

function SkillDetail({
  skill,
  onOpen,
  onEnable,
  onDisable,
  onResolve,
  onReadEnv,
  onSaveEnv,
}: {
  skill?: skillmgr.Skill;
  onOpen: (path: string) => void;
  onEnable: (skillId: string) => void;
  onDisable: (skill: skillmgr.Skill) => void;
  onResolve: (skillId: string) => void;
  onReadEnv: (skillId: string) => Promise<string>;
  onSaveEnv: (skillId: string, content: string) => Promise<void>;
}) {
  if (!skill) {
    return <div className="p-5 text-sm text-muted-foreground">No skill selected.</div>;
  }
  return (
    <div className="h-full overflow-y-auto p-5">
      <div className="mb-5 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="truncate text-xl font-semibold">{skill.name}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{skill.description || skill.sourceAlias}</p>
        </div>
        <StatusPill status={skill.status} />
      </div>

      <DetailSection title="Paths">
        <PathRow label="Source folder" path={skill.sourcePath} onOpen={onOpen} />
        <PathRow label="Link path" path={skill.symlinkPath} onOpen={onOpen} />
        {skill.symlinkTarget && !skill.isActive && (
          <ReadOnlyRow label="Currently points to" value={skill.symlinkTarget} />
        )}
      </DetailSection>

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
                <ChevronRight className="h-4 w-4 shrink-0" />
              </button>
            ))}
          </div>
        </DetailSection>
      )}

      <DetailSection title="Files">
        <div className="flex flex-wrap gap-2">
          {(skill.files ?? []).map((file) => (
            <span key={file} className="rounded-md border border-border bg-slate-50 px-2 py-1 text-xs">
              {file}
            </span>
          ))}
        </div>
      </DetailSection>

      {skill.preview && (
        <DetailSection title={`Preview: ${skill.previewFile}`}>
          <pre className="max-h-72 overflow-auto rounded-md border border-border bg-slate-950 p-3 text-xs leading-5 text-slate-100">
            {skill.preview}
          </pre>
        </DetailSection>
      )}

      <EnvEditor skill={skill} onReadEnv={onReadEnv} onSaveEnv={onSaveEnv} />

      <div className="sticky bottom-0 mt-5 flex flex-wrap gap-2 border-t border-border bg-white pt-4">
        <Button variant="outline" onClick={() => onOpen(skill.sourcePath)}>
          <Folder className="h-4 w-4" />
          Open
        </Button>
        {isActiveSkill(skill) ? (
          <Button variant="outline" onClick={() => onDisable(skill)}>
            Disable
          </Button>
        ) : skill.status === "conflict" ? (
          <Button onClick={() => onResolve(skill.id)}>Apply</Button>
        ) : (
          <Button onClick={() => onEnable(skill.id)} disabled={["invalid", "error"].includes(skill.status)}>
            Enable
          </Button>
        )}
      </div>
    </div>
  );
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
    <DetailSection title="Preview: .env">
      <div className="rounded-md border border-border bg-white">
        <textarea
          value={content}
          onChange={(event) => setContent(event.target.value)}
          spellCheck={false}
          placeholder="KEY=value"
          className="min-h-40 w-full resize-y border-0 bg-slate-950 p-3 font-mono text-xs leading-5 text-slate-100 outline-none placeholder:text-slate-500"
        />
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border bg-slate-50 px-3 py-2">
          <span className="text-xs text-muted-foreground">
            {loading ? "Loading .env..." : dirty ? "Unsaved changes" : ".env saved"}
          </span>
          <div className="flex gap-2">
            <Button variant="outline" onClick={reload} disabled={loading || saving}>
              Reload
            </Button>
            <Button onClick={save} disabled={loading || saving || !dirty}>
              {saving ? "Saving" : "Save .env"}
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
  onSave,
}: {
  inventory: skillmgr.Inventory;
  onClose: () => void;
  onSave: (config: skillmgr.Config) => Promise<void>;
}) {
  const [config, setConfig] = useState(() => skillmgr.Config.createFrom(inventory.config));
  const updateConfig = (next: Partial<skillmgr.Config>) => {
    setConfig(skillmgr.Config.createFrom({ ...config, ...next }));
  };
  const updateScan = (next: Partial<skillmgr.ScanConfig>) => {
    updateConfig({ scan: skillmgr.ScanConfig.createFrom({ ...config.scan, ...next }) });
  };

  return (
    <Modal title="Settings" onClose={onClose}>
      <div className="space-y-4">
        <label className="block text-sm font-medium">
          Target skill directory
          <input
            value={config.targetDir}
            onChange={(event) => updateConfig({ targetDir: event.target.value })}
            className="mt-2 h-9 w-full rounded-md border border-input px-3 text-sm"
          />
        </label>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label className="flex items-center gap-2 rounded-md border border-border p-3 text-sm">
            <input
              type="checkbox"
              checked={config.scan.autoRescanOnStartup}
              onChange={(event) => updateScan({ autoRescanOnStartup: event.target.checked })}
            />
            Auto rescan on startup
          </label>
          <label className="flex items-center gap-2 rounded-md border border-border p-3 text-sm">
            <input
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
            A folder is shown as a skill only when it contains `SKILL.md`.
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={() => onSave(config)}>Save</Button>
        </div>
      </div>
    </Modal>
  );
}

function StatusPill({ status }: { status: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs font-medium",
        statusClass[status] ?? statusClass.disabled,
      )}
    >
      {status === "synced" && <Check className="h-3 w-3" />}
      {status === "conflict" && <AlertTriangle className="h-3 w-3" />}
      {status === "syncing" && <Loader2 className="h-3 w-3 animate-spin" />}
      {statusLabels[status] ?? status}
    </span>
  );
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mb-5">
      <h3 className="mb-2 text-xs font-semibold uppercase tracking-normal text-muted-foreground">{title}</h3>
      {children}
    </section>
  );
}

function PathRow({ label, path, onOpen }: { label: string; path: string; onOpen: (path: string) => void }) {
  return (
    <div className="mb-2 rounded-md border border-border p-2">
      <div className="mb-1 flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span>{label}</span>
        <button className="rounded p-1 hover:bg-slate-100" onClick={() => onOpen(path)} title={`Open ${label}`}>
          <ExternalLink className="h-3.5 w-3.5" />
        </button>
      </div>
      <div className="break-all font-mono text-xs text-slate-700">{path}</div>
    </div>
  );
}

function ReadOnlyRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="mb-2 rounded-md border border-border p-2">
      <div className="mb-1 text-xs text-muted-foreground">{label}</div>
      <div className="break-all font-mono text-xs text-slate-700">{value}</div>
    </div>
  );
}

function IssueLine({ value }: { value: string }) {
  return (
    <div className="rounded-md border border-rose-200 bg-rose-50 p-2 text-sm text-rose-700">{value}</div>
  );
}

function PanelHeader({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <div className="flex h-14 items-center justify-between border-b border-border px-4">
      <h2 className="text-sm font-semibold">{title}</h2>
      {children}
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
        "inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition disabled:pointer-events-none disabled:opacity-50",
        variant === "default" && "bg-primary text-primary-foreground hover:bg-blue-600",
        variant === "outline" && "border border-border bg-white text-foreground hover:bg-slate-50",
        variant === "ghost" && "hover:bg-slate-100",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

function IconButton({ className, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      className={cn(
        "inline-flex h-9 w-9 items-center justify-center rounded-md border border-border bg-white text-slate-700 transition hover:bg-slate-50 disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

function SmallAction({ title, children, onClick }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      title={title}
      onClick={onClick}
      className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-white hover:bg-slate-50"
    >
      {children}
    </button>
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/35 p-4">
      <div className="w-full max-w-xl rounded-lg border border-border bg-white shadow-xl">
        <div className="flex h-14 items-center justify-between border-b border-border px-5">
          <h2 className="text-base font-semibold">{title}</h2>
          <IconButton title="Close" onClick={onClose}>
            <X className="h-4 w-4" />
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

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.round(value)));
}

export default App;
