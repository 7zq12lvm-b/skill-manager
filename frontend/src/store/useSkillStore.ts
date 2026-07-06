import { create } from "zustand";
import { skillmgr } from "../../wailsjs/go/models";
import {
  AddSource,
  AdoptCurrentEnabledSkills,
  ApplySync,
  BrowseForSource,
  BrowseForSyncFolder,
  BrowseForTarget,
  CloneRepository,
  DisableSkill,
  EnableSkill,
  EnableSkillLocalOnly,
  GetInventory,
  OpenInVSCode,
  OpenPath,
  PullRepository,
  PullSource,
  ReadSkillEnvFile,
  RemoveSource,
  RemoveSkillFromSync,
  RenameRepository,
  RenameSource,
  RescanAll,
  ResolveConflict,
  SaveConfig,
  SaveSkillEnvFile,
  SaveSkillTags,
} from "../../wailsjs/go/main/App";

type StatusFilter = "all" | string;

type SkillStore = {
  inventory?: skillmgr.Inventory;
  selectedSkillId?: string;
  selectedSourceId: string;
  statusFilter: StatusFilter;
  query: string;
  loading: boolean;
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
  enableSkillLocalOnly: (skillId: string) => Promise<void>;
  disableSkill: (skillId: string) => Promise<void>;
  removeSkillFromSync: (skillId: string) => Promise<void>;
  applySync: () => Promise<void>;
  adoptCurrentEnabledSkills: () => Promise<void>;
  cloneRepository: (repoId: string, cloneUrl: string, parentDir: string, folderName: string) => Promise<void>;
  resolveConflict: (skillId: string) => Promise<void>;
  saveConfig: (config: skillmgr.Config) => Promise<void>;
  readSkillEnv: (skillId: string) => Promise<string>;
  saveSkillEnv: (skillId: string, content: string) => Promise<void>;
  saveSkillTags: (skillId: string, tags: string[]) => Promise<void>;
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
) {
  set({ loading: true, error: undefined });
  try {
    const inventory = await action();
    set({ inventory, loading: false });
  } catch (error) {
    set({ error: error instanceof Error ? error.message : String(error), loading: false });
  }
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
  load: async () => runWithInventory(set, GetInventory),
  rescan: async () => runWithInventory(set, RescanAll),
  addSource: async (path) => runWithInventory(set, () => AddSource(path)),
  browseAndAddSource: async () => {
    set({ loading: true, error: undefined });
    try {
      const path = await BrowseForSource();
      if (!path) {
        set({ loading: false });
        return;
      }
      const inventory = await AddSource(path);
      set({ inventory, loading: false });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
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
      return await BrowseForSource();
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return "";
    }
  },
  removeSource: async (sourceId) => runWithInventory(set, () => RemoveSource(sourceId)),
  renameSource: async (sourceId, alias) =>
    runWithInventory(set, () => RenameRepository(sourceId, alias).catch(() => RenameSource(sourceId, alias))),
  pullSource: async (sourceId) => {
    set({ loading: true, error: undefined });
    try {
      const result = await PullSource(sourceId);
      set((state) => ({
        inventory: result.inventory,
        loading: false,
        pullResults: { ...state.pullResults, [sourceId]: summarizePullMessage(result.message) },
      }));
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
    }
  },
  pullRepository: async (repoId) => {
    set({ loading: true, error: undefined });
    try {
      const result = await PullRepository(repoId).catch(() => PullSource(repoId));
      set((state) => ({
        inventory: result.inventory,
        loading: false,
        pullResults: { ...state.pullResults, [repoId]: summarizePullMessage(result.message) },
      }));
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
    }
  },
  enableSkill: async (skillId) => runWithInventory(set, () => EnableSkill(skillId)),
  enableSkillLocalOnly: async (skillId) => runWithInventory(set, () => EnableSkillLocalOnly(skillId)),
  disableSkill: async (skillId) => runWithInventory(set, () => DisableSkill(skillId)),
  removeSkillFromSync: async (skillId) => runWithInventory(set, () => RemoveSkillFromSync(skillId)),
  applySync: async () => {
    set({ loading: true, error: undefined });
    try {
      const result = await ApplySync();
      set({ inventory: result.inventory, loading: false, pullResults: { ...get().pullResults, sync: result.message } });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
    }
  },
  adoptCurrentEnabledSkills: async () => {
    set({ loading: true, error: undefined });
    try {
      const result = await AdoptCurrentEnabledSkills();
      const skipped = result.skipped?.length ? `, skipped ${result.skipped.length}` : "";
      set({
        inventory: result.inventory,
        loading: false,
        pullResults: { ...get().pullResults, sync: `Adopted ${result.adopted}${skipped}` },
      });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
    }
  },
  cloneRepository: async (repoId, cloneUrl, parentDir, folderName) => {
    set({ loading: true, error: undefined });
    try {
      const result = await CloneRepository(repoId, cloneUrl, parentDir, folderName);
      set({ inventory: result.inventory, loading: false, pullResults: { ...get().pullResults, [repoId]: result.message } });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
    }
  },
  resolveConflict: async (skillId) => runWithInventory(set, () => ResolveConflict(skillId)),
  saveConfig: async (config) => runWithInventory(set, () => SaveConfig(config)),
  readSkillEnv: async (skillId) => {
    try {
      return await ReadSkillEnvFile(skillId);
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error) });
      return "";
    }
  },
  saveSkillEnv: async (skillId, content) => {
    set({ loading: true, error: undefined });
    try {
      const inventory = await SaveSkillEnvFile(skillId, content);
      set({ inventory, loading: false });
    } catch (error) {
      set({ error: error instanceof Error ? error.message : String(error), loading: false });
      throw error;
    }
  },
  saveSkillTags: async (skillId, tags) =>
    runWithInventory(set, () => SaveSkillTags(skillId, tags)),
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
  selectSkill: (skillId) => set({ selectedSkillId: skillId }),
  setSelectedSourceId: (sourceId) => set({ selectedSourceId: sourceId }),
  setStatusFilter: (status) => set({ statusFilter: status }),
  setQuery: (query) => set({ query }),
  clearError: () => set({ error: undefined }),
}));
