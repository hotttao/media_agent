import type {
  AppDefinition,
  AppInstance,
  ProfileBackend,
  SlotConfig,
  SlotProfile,
  SlotState,
} from "@/foundation/slots/types";

function defaultSlot(config: SlotConfig): SlotState {
  const opened = config.defaultOpened.map((appId) => ({
    id: `${config.id}:${appId}`,
    appId,
  }));
  return { opened, activeInstanceId: opened[0]?.id ?? null };
}

export function normalizeProfile(
  value: unknown,
  configs: SlotConfig[],
  apps: AppDefinition[],
): SlotProfile {
  const registered = new Set(apps.map((app) => app.id));
  const raw =
    value && typeof value === "object" ? (value as Partial<SlotProfile>) : {};
  const rawSlots = raw.slots && typeof raw.slots === "object" ? raw.slots : {};
  const slots: Record<string, SlotState> = {};
  const globallyOpened = new Set<string>();
  for (const config of configs) {
    const candidate = rawSlots[config.id];
    const source =
      candidate && Array.isArray(candidate.opened)
        ? candidate.opened
        : defaultSlot(config).opened;
    const opened = source.filter((item): item is AppInstance => {
      if (
        !item ||
        typeof item.id !== "string" ||
        typeof item.appId !== "string" ||
        !registered.has(item.appId)
      )
        return false;
      const app = apps.find((definition) => definition.id === item.appId);
      if (!app?.allowMultiple && globallyOpened.has(item.appId)) return false;
      globallyOpened.add(item.appId);
      return true;
    });
    const activeInstanceId = opened.some(
      (item) => item.id === candidate?.activeInstanceId,
    )
      ? (candidate?.activeInstanceId ?? null)
      : (opened[0]?.id ?? null);
    slots[config.id] = { opened, activeInstanceId };
  }
  const layoutId = raw.layoutId === "1col" ? "1col" : "3col";
  const activeSlotId =
    typeof raw.activeSlotId === "string" && slots[raw.activeSlotId]
      ? raw.activeSlotId
      : "slot-c";
  return {
    version: 2,
    layoutId,
    activeSlotId,
    slots,
    instances:
      raw.instances && typeof raw.instances === "object" ? raw.instances : {},
  };
}

export function profileKey(workspaceId: string): string {
  return `media-agent:ui-profile:v2:${workspaceId}`;
}

export const localStorageProfileBackend: ProfileBackend = {
  load(workspaceId) {
    const current = localStorage.getItem(profileKey(workspaceId));
    const legacy = localStorage.getItem(
      `media-agent:ui-profile:v1:${workspaceId}`,
    );
    try {
      return JSON.parse(current ?? legacy ?? "null");
    } catch {
      return null;
    }
  },
  save(workspaceId, profile) {
    localStorage.setItem(profileKey(workspaceId), JSON.stringify(profile));
  },
};
