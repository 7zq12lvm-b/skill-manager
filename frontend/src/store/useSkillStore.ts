import { create } from "zustand";
import { skillmgr } from "../../wailsjs/go/models";
import {
  AddSource,
  BrowseForSource,
  BrowseForTarget,
  DisableSkill,
  EnableSkill,
  GetInventory,
  OpenInVSCode,
  OpenPath,
  ReadSkillEnvFile,
  RemoveSource,
  RenameSource,
  RescanAll,
  ResolveConflict,
  SaveConfig,
  SaveSkillEnvFile,
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
  setInventory: (inventory: skillmgr.Inventory) => void;
  load: () => Promise<void>;
  rescan: () => Promise<void>;
  addSource: (path: string) => Promise<void>;
  browseAndAddSource: () => Promise<void>;
  browseForTarget: () => Promise<string>;
  removeSource: (sourceId: string) => Promise<void>;
  renameSource: (sourceId: string, alias: string) => Promise<void>;
  enableSkill: (skillId: string) => Promise<void>;
  disableSkill: (skillId: string) => Promise<void>;
  resolveConflict: (skillId: string) => Promise<void>;
  saveConfig: (config: skillmgr.Config) => Promise<void>;
  readSkillEnv: (skillId: string) => Promise<string>;
  saveSkillEnv: (skillId: string, content: string) => Promise<void>;
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

export const useSkillStore = create<SkillStore>((set, get) => ({
  selectedSourceId: "all",
  statusFilter: "all",
  query: "",
  loading: false,
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
  removeSource: async (sourceId) => runWithInventory(set, () => RemoveSource(sourceId)),
  renameSource: async (sourceId, alias) =>
    runWithInventory(set, () => RenameSource(sourceId, alias)),
  enableSkill: async (skillId) => runWithInventory(set, () => EnableSkill(skillId)),
  disableSkill: async (skillId) => runWithInventory(set, () => DisableSkill(skillId)),
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
