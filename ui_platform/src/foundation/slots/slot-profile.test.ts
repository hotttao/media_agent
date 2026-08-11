import { describe, expect, it } from "vitest";
import { normalizeProfile } from "./slot-profile";

const Component = () => null;
const apps = [{ id: "chat", label: "Chat", Component }];
const configs = [
  { id: "slot-a", defaultOpened: ["chat"] },
  { id: "slot-b", defaultOpened: ["chat"] },
  { id: "slot-c", defaultOpened: [] },
];

describe("normalizeProfile", () => {
  it("creates defaults and removes unavailable apps", () => {
    expect(
      normalizeProfile(null, configs, apps).slots["slot-a"].activeInstanceId,
    ).toBe("slot-a:chat");
    expect(
      normalizeProfile(null, configs, apps).slots["slot-b"].opened,
    ).toEqual([]);
    const value = normalizeProfile(
      {
        slots: {
          "slot-a": {
            opened: [{ id: "x", appId: "removed" }],
            activeInstanceId: "x",
          },
        },
      },
      configs,
      apps,
    );
    expect(value.version).toBe(2);
    expect(value.layoutId).toBe("3col");
    expect(value.activeSlotId).toBe("slot-c");
    expect(value.slots["slot-a"]).toEqual({
      opened: [],
      activeInstanceId: null,
    });
  });
});
