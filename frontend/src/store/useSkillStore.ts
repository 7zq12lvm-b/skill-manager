import { create } from "zustand";
import { skillmgr } from "../../wailsjs/go/models";
import {
  AddSkillTags,
  AddSource,
  BrowseForCloneParent,
  BrowseForExistingRepository,
  BrowseForSource,
  BrowseForSyncFolder,
  BrowseForTarget,
  CloneRepository,
  DisableSkill,
  DisableSkills,
  EnableSkill,
  EnableSkills,
  GenerateSkillProfile,
  GetInventory,
  ListSkillFiles,
  OpenInTerminal,
  OpenInVSCode,
  OpenPath,
  PullRepository,
  PullSource,
  ReadSkillEnvFile,
  ReadSkillFilePreview,
  RemoveMissingSkill,
  RemoveSource,
  RenameRepository,
  RenameSource,
  RescanAll,
  ResolveConflict,
  SaveConfig,
  SaveLLMConfig,
  SaveSkillEnvFile,
  SaveSkillNote,
  SaveSkillTags,
  UseExistingRepository,
} from "../../wailsjs/go/main/App";

type StatusFilter = "all" | string;

type SkillStore = {
  inventory?: skillmgr.Inventory;
  selectedSkillId?: string;
  selectedSourceId: string;
  statusFilter: StatusFilter;
  query: string;
  loading: boolean;
  loadingLabel?: string;
  error?: string;
  pullResults: Record<string, string>;
  setInventory: (inventory: skillmgr.Inventory) => void;
  load: () => Promise<void>;
  rescan: () => Promise<void>;
  addSource: (path: string) => Promise<void>;
  browseAndAddSource: () => Promise<void>;
  browseForTarget: () => Promise<string>;
  browseForSyncFolder: () => Promise<string>;
  browseForRepositoryFolder: () => Promise<string>;
  removeSource: (sourceId: string) => Promise<void>;
  renameSource: (sourceId: string, alias: string) => Promise<void>;
  pullSource: (sourceId: string) => Promise<void>;
  pullRepository: (repoId: string) => Promise<void>;
  enableSkill: (skillId: string) => Promise<void>;
  enableSkills: (skillIds: string[]) => Promise<void>;
  disableSkill: (skillId: string) => Promise<void>;
  disableSkills: (skillIds: string[]) => Promise<void>;
  cloneRepository: (repoId: string, cloneUrl: string, parentDir: string, folderName: string) => Promise<boolean>;
  useExistingRepository: (repoId: string) => Promise<void>;
  resolveConflict: (skillId: string) => Promise<void>;
  saveConfig: (config: skillmgr.Config) => Promise<void>;
  saveLLMConfig: (config: skillmgr.SyncLLMConfig) => Promise<void>;
  generateSkillProfile: (skillId: string, force?: boolean) => Promise<void>;
  readSkillEnv: (skillId: string) => Promise<string>;
  saveSkillEnv: (skillId: string, content: string) => Promise<void>;
  saveSkillNote: (skillId: string, note: string) => Promise<void>;
  saveSkillTags: (skillId: string, tags: string[]) => Promise<void>;
  addSkillTags: (skillIds: string[], tags: string[]) => Promise<void>;
  removeMissingSkill: (skillId: string) => Promise<void>;
  listSkillFiles: (skillId: string, relativeDir: string) => Promise<skillmgr.SkillFileEntry[]>;
  readSkillFilePreview: (skillId: string, relativeFile: string) => Promise<skillmgr.SkillFilePreview>;
  openInTerminal: (path: string) => Promise<void>;
  openInVSCode: (path: string) => Promise<void>;
  openPath: (path: string) => Promise<void>;
  selectSkill: (skillId?: string) => void;
  setSelectedSourceId: (sourceId: string) => void;
  setStatusFilter: (status: StatusFilter) => void;
  setQuery: (query: string) => void;
  clearError: () => void;
};

async function runWithInventory(
  set: (partial: Partial<SkillStore>) => void,
  action: () => Promise<skillmgr.Inventory>,
  loadingLabel = "Working...",
) {
  set({ loading: true, loadingLabel, error: undefined });
  await waitForPaint();
  try {
    const inventory = await action();
    set({ inventory, loading: false, loadingLabel: undefined });
  } catch (error) {
    set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
  }
}

function waitForPaint() {
  return new Promise<void>((resolve) => {
    if (typeof window === "undefined" || typeof window.requestAnimationFrame !== "function") {
      setTimeout(resolve, 0);
      return;
    }
    window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => resolve());
    });
  });
}

function summarizePullMessage(message: string) {
  const lines = message
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const alreadyCurrent = lines.find((line) => /^Already up to date\.?$/i.test(line));
  if (alreadyCurrent) {
    return "Already up to date.";
  }
  const updateRange = lines.find((line) => /^Updating\s+\S+\.\.\S+\.?$/i.test(line));
  const fastForward = lines.find((line) => /^Fast-forward\b/i.test(line));
  if (fastForward && updateRange) {
    return `Fast-forwarded ${updateRange.replace(/^Updating\s+/i, "").replace(/\.$/, "")}.`;
  }
  if (fastForward) {
    return "Pull completed with a fast-forward.";
  }
  if (updateRange) {
    return `Updated ${updateRange.replace(/^Updating\s+/i, "").replace(/\.$/, "")}.`;
  }
  const usefulLine =
    lines.find((line) => /^From\b/i.test(line)) ??
    lines[0];
  return usefulLine || "Pull completed.";
}

export const useSkillStore = create<SkillStore>((set, get) => ({
  selectedSourceId: "all",
  statusFilter: "all",
  query: "",
  loading: false,
  pullResults: {},
  setInventory: (inventory) => {
    const selectedSkillId = get().selectedSkillId;
    const stillExists = inventory.skills?.some((skill) => skill.id === selectedSkillId);
    set({
      inventory,
      selectedSkillId: stillExists ? selectedSkillId : inventory.skills?.[0]?.id,
    });
  },
  load: async () => runWithInventory(set, GetInventory, "Loading inventory..."),
  rescan: async () => runWithInventory(set, RescanAll, "Scanning skills..."),
  addSource: async (path) => runWithInventory(set, () => AddSource(path), "Scanning repository..."),
  browseAndAddSource: async () => {
    set({ error: undefined });
    try {
      const path = await BrowseForSource();
      if (!path) {
        return;
      }
      set({ loading: true, loadingLabel: "Scanning repository...", error: undefined });
      await waitForPaint();
      const inventory = await AddSource(path);
      set({ inventory, loading: false, loadingLabel: undefined });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  browseForTarget: async () => {
    try {
      return await BrowseForTarget();
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return "";
    }
  },
  browseForSyncFolder: async () => {
    try {
      return await BrowseForSyncFolder();
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return "";
    }
  },
  browseForRepositoryFolder: async () => {
    try {
      return await BrowseForCloneParent();
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return "";
    }
  },
  removeSource: async (sourceId) => runWithInventory(set, () => RemoveSource(sourceId), "Updating repositories..."),
  renameSource: async (sourceId, alias) =>
    runWithInventory(set, () => RenameRepository(sourceId, alias).catch(() => RenameSource(sourceId, alias)), "Saving alias..."),
  pullSource: async (sourceId) => {
    set({ loading: true, loadingLabel: "Pulling repository...", error: undefined });
    await waitForPaint();
    try {
      const result = await PullSource(sourceId);
      set((state) => ({
        inventory: result.inventory,
        loading: false,
        loadingLabel: undefined,
        pullResults: { ...state.pullResults, [sourceId]: summarizePullMessage(result.message) },
      }));
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  pullRepository: async (repoId) => {
    set({ loading: true, loadingLabel: "Pulling repository...", error: undefined });
    await waitForPaint();
    try {
      const result = await PullRepository(repoId).catch(() => PullSource(repoId));
      set((state) => ({
        inventory: result.inventory,
        loading: false,
        loadingLabel: undefined,
        pullResults: { ...state.pullResults, [repoId]: summarizePullMessage(result.message) },
      }));
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  enableSkill: async (skillId) => runWithInventory(set, () => EnableSkill(skillId), "Enabling skill..."),
  enableSkills: async (skillIds) => {
    set({ loading: true, loadingLabel: "Enabling listed skills...", error: undefined });
    await waitForPaint();
    try {
      const result = await EnableSkills(skillIds);
      const failed = result.failed?.length ? `, ${result.failed.length} failed` : "";
      set({
        inventory: result.inventory,
        loading: false,
        loadingLabel: undefined,
        pullResults: {
          ...get().pullResults,
          bulk: `Enabled ${result.enabled}, already enabled ${result.alreadyEnabled}, skipped ${result.skipped}${failed}.`,
        },
      });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  disableSkill: async (skillId) => runWithInventory(set, () => DisableSkill(skillId), "Disabling skill..."),
  disableSkills: async (skillIds) => {
    set({ loading: true, loadingLabel: "Disabling selected skills...", error: undefined });
    await waitForPaint();
    try {
      const result = await DisableSkills(skillIds);
      const failed = result.failed?.length ? `, ${result.failed.length} failed` : "";
      set({
        inventory: result.inventory,
        loading: false,
        loadingLabel: undefined,
        pullResults: {
          ...get().pullResults,
          bulk: `Disabled ${result.disabled}, already disabled ${result.alreadyDisabled}, skipped ${result.skipped}${failed}.`,
        },
      });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  cloneRepository: async (repoId, cloneUrl, parentDir, folderName) => {
    set({ loading: true, loadingLabel: "Cloning repository...", error: undefined });
    await waitForPaint();
    try {
      const result = await CloneRepository(repoId, cloneUrl, parentDir, folderName);
      set({
        inventory: result.inventory,
        loading: false,
        loadingLabel: undefined,
        pullResults: { ...get().pullResults, [repoId]: result.message },
      });
      return true;
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
      return false;
    }
  },
  useExistingRepository: async (repoId) => {
    set({ error: undefined });
    try {
      const path = await BrowseForExistingRepository();
      if (!path) return;
      set({ loading: true, loadingLabel: "Linking existing repository...", error: undefined });
      await waitForPaint();
      const inventory = await UseExistingRepository(repoId, path);
      set({ inventory, loading: false, loadingLabel: undefined });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  resolveConflict: async (skillId) => runWithInventory(set, () => ResolveConflict(skillId), "Resolving conflict..."),
  saveConfig: async (config) => runWithInventory(set, () => SaveConfig(config), "Saving settings..."),
  saveLLMConfig: async (config) => runWithInventory(set, () => SaveLLMConfig(config), "Saving LLM settings..."),
  generateSkillProfile: async (skillId, force = false) => {
    set({ error: undefined });
    try {
      const result = await GenerateSkillProfile(skillId, force);
      set({ inventory: result.inventory });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
    }
  },
  readSkillEnv: async (skillId) => {
    try {
      return await ReadSkillEnvFile(skillId);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return "";
    }
  },
  saveSkillEnv: async (skillId, content) => {
    set({ loading: true, loadingLabel: "Saving .env...", error: undefined });
    await waitForPaint();
    try {
      const inventory = await SaveSkillEnvFile(skillId, content);
      set({ inventory, loading: false, loadingLabel: undefined });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
      throw error;
    }
  },
  saveSkillNote: async (skillId, note) => {
    set({ error: undefined });
    try {
      const inventory = await SaveSkillNote(skillId, note);
      get().setInventory(inventory);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  },
  saveSkillTags: async (skillId, tags) =>
    runWithInventory(set, () => SaveSkillTags(skillId, tags), "Saving tags..."),
  addSkillTags: async (skillIds, tags) => {
    set({ loading: true, loadingLabel: "Adding tags to selected skills...", error: undefined });
    await waitForPaint();
    try {
      const result = await AddSkillTags(skillIds, tags);
      const failed = result.failed?.length ? `, ${result.failed.length} failed` : "";
      set({
        inventory: result.inventory,
        loading: false,
        loadingLabel: undefined,
        pullResults: {
          ...get().pullResults,
          bulk: `Tagged ${result.updated}, already tagged ${result.unchanged}, skipped ${result.skipped}${failed}.`,
        },
      });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
    }
  },
  removeMissingSkill: async (skillId) => {
    set({ loading: true, loadingLabel: "Removing missing skill...", error: undefined });
    await waitForPaint();
    try {
      const inventory = await RemoveMissingSkill(skillId);
      get().setInventory(inventory);
      set({ loading: false, loadingLabel: undefined });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false, loadingLabel: undefined });
      throw error;
    }
  },
  listSkillFiles: async (skillId, relativeDir) => {
    try {
      set({ error: undefined });
      return await ListSkillFiles(skillId, relativeDir);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return [];
    }
  },
  readSkillFilePreview: async (skillId, relativeFile) => {
    try {
      return await ReadSkillFilePreview(skillId, relativeFile);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      throw error;
    }
  },
  openPath: async (path) => {
    try {
      await OpenPath(path);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
    }
  },
  openInVSCode: async (path) => {
    try {
      await OpenInVSCode(path);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
    }
  },
  openInTerminal: async (path) => {
    try {
      await OpenInTerminal(path);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
    }
  },
  selectSkill: (skillId) => set({ selectedSkillId: skillId }),
  setSelectedSourceId: (sourceId) => set({ selectedSourceId: sourceId }),
  setStatusFilter: (status) => set({ statusFilter: status }),
  setQuery: (query) => set({ query }),
  clearError: () => set({ error: undefined }),
}));
