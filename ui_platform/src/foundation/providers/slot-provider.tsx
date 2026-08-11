import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import {
  localStorageProfileBackend,
  normalizeProfile,
} from "@/foundation/slots/slot-profile";
import type {
  AppDefinition,
  AppInstance,
  ProfileBackend,
  SlotConfig,
  SlotProfile,
} from "@/foundation/slots/types";

interface SlotContextValue {
  apps: AppDefinition[];
  configs: SlotConfig[];
  profile: SlotProfile;
  setLayout(layoutId: "1col" | "3col"): void;
  setActiveSlot(slotId: string): void;
  openApp(slotId: string, appId: string): void;
  ensureInstance(
    appId: string,
    fallbackSlotId?: string,
  ): { slotId: string; instanceId: string } | null;
  activateInstance(slotId: string, instanceId: string): void;
  closeInstance(slotId: string, instanceId: string): void;
  filledSlot: string | null;
  setFilledSlot(slotId: string | null): void;
}

const SlotContext = createContext<SlotContextValue | null>(null);

export function SlotProvider({
  workspaceId,
  apps,
  configs,
  backend = localStorageProfileBackend,
  children,
}: {
  workspaceId: string;
  apps: AppDefinition[];
  configs: SlotConfig[];
  backend?: ProfileBackend;
  children: ReactNode;
}) {
  const initial = useMemo(
    () =>
      normalizeProfile(
        workspaceId ? backend.load(workspaceId) : null,
        configs,
        apps,
      ),
    [workspaceId, apps, configs, backend],
  );
  const [profile, setProfile] = useState(initial);
  const [filledSlot, setFilledSlot] = useState<string | null>(null);
  useEffect(() => {
    setProfile(initial);
    setFilledSlot(null);
  }, [initial]);
  useEffect(() => {
    if (workspaceId) backend.save(workspaceId, profile);
  }, [backend, profile, workspaceId]);

  const setLayout = useCallback(
    (layoutId: "1col" | "3col") =>
      setProfile((current) => ({ ...current, layoutId })),
    [],
  );
  const setActiveSlot = useCallback(
    (activeSlotId: string) =>
      setProfile((current) =>
        current.slots[activeSlotId] ? { ...current, activeSlotId } : current,
      ),
    [],
  );
  const locateApp = useCallback((current: SlotProfile, appId: string) => {
    for (const [slotId, slot] of Object.entries(current.slots)) {
      const found = slot.opened.find((item) => item.appId === appId);
      if (found) return { slotId, instance: found };
    }
    return null;
  }, []);
  const openApp = useCallback(
    (slotId: string, appId: string) => {
      const definition = apps.find((app) => app.id === appId);
      if (!definition) return;
      setProfile((current) => {
        const slot = current.slots[slotId];
        if (!slot) return current;
        const globalExisting = !definition.allowMultiple
          ? locateApp(current, appId)
          : null;
        if (globalExisting)
          return {
            ...current,
            activeSlotId: globalExisting.slotId,
            slots: {
              ...current.slots,
              [globalExisting.slotId]: {
                ...current.slots[globalExisting.slotId],
                activeInstanceId: globalExisting.instance.id,
              },
            },
          };
        const next: AppInstance = {
          id: `${slotId}:${appId}:${crypto.randomUUID()}`,
          appId,
        };
        return {
          ...current,
          activeSlotId: slotId,
          slots: {
            ...current.slots,
            [slotId]: {
              opened: [...slot.opened, next],
              activeInstanceId: next.id,
            },
          },
        };
      });
    },
    [apps, locateApp],
  );
  const ensureInstance = useCallback(
    (appId: string, fallbackSlotId = configs[0]?.id) => {
      const existing = locateApp(profile, appId);
      if (existing) {
        setProfile((current) => ({
          ...current,
          activeSlotId: existing.slotId,
          slots: {
            ...current.slots,
            [existing.slotId]: {
              ...current.slots[existing.slotId],
              activeInstanceId: existing.instance.id,
            },
          },
        }));
        return { slotId: existing.slotId, instanceId: existing.instance.id };
      }
      const preferred =
        configs.find((config) => config.defaultOpened.includes(appId))?.id ??
        fallbackSlotId;
      if (!preferred) return null;
      const id = `${preferred}:${appId}:${crypto.randomUUID()}`;
      setProfile((current) => ({
        ...current,
        activeSlotId: preferred,
        slots: {
          ...current.slots,
          [preferred]: {
            opened: [...current.slots[preferred].opened, { id, appId }],
            activeInstanceId: id,
          },
        },
      }));
      return { slotId: preferred, instanceId: id };
    },
    [configs, locateApp, profile],
  );
  const activateInstance = useCallback(
    (slotId: string, instanceId: string) =>
      setProfile((current) =>
        current.slots[slotId]?.opened.some((item) => item.id === instanceId)
          ? {
              ...current,
              activeSlotId: slotId,
              slots: {
                ...current.slots,
                [slotId]: {
                  ...current.slots[slotId],
                  activeInstanceId: instanceId,
                },
              },
            }
          : current,
      ),
    [],
  );
  const closeInstance = useCallback(
    (slotId: string, instanceId: string) =>
      setProfile((current) => {
        const slot = current.slots[slotId];
        if (!slot) return current;
        const index = slot.opened.findIndex((item) => item.id === instanceId);
        if (index < 0) return current;
        const opened = slot.opened.filter((item) => item.id !== instanceId);
        const activeInstanceId =
          slot.activeInstanceId === instanceId
            ? (opened[index - 1]?.id ?? opened[0]?.id ?? null)
            : slot.activeInstanceId;
        if (!opened.length)
          setFilledSlot((filled) => (filled === slotId ? null : filled));
        const instances = { ...current.instances };
        delete instances[instanceId];
        return {
          ...current,
          instances,
          slots: { ...current.slots, [slotId]: { opened, activeInstanceId } },
        };
      }),
    [],
  );
  const value = useMemo(
    () => ({
      apps,
      configs,
      profile,
      setLayout,
      setActiveSlot,
      openApp,
      ensureInstance,
      activateInstance,
      closeInstance,
      filledSlot,
      setFilledSlot,
    }),
    [
      apps,
      configs,
      profile,
      setLayout,
      setActiveSlot,
      openApp,
      ensureInstance,
      activateInstance,
      closeInstance,
      filledSlot,
    ],
  );
  return <SlotContext.Provider value={value}>{children}</SlotContext.Provider>;
}

export function useSlots(): SlotContextValue {
  const value = useContext(SlotContext);
  if (!value) throw new Error("useSlots must be used within SlotProvider");
  return value;
}
