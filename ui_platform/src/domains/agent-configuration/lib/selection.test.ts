import { describe, expect, it } from "vitest";
import { selectedPrompt, selectionBody } from "./selection";

describe("explicit Agent selection", () => {
  it("switches prompt modes without retaining a hidden library reference or stale checksum", () => {
    const refs = [
      { kind: "media_control.configured_prompt", id: "old", version: 1 },
      {
        kind: "media_control.skill",
        id: "skill",
        version: 2,
        checksum: "old-checksum",
      },
      { kind: "media_control.memory_store", id: "memory", version: 4 },
    ];
    expect(
      selectionBody(
        {
          mode: "custom",
          content: "direct",
          resource_id: "old",
          resource_version: 1,
        },
        refs,
      ),
    ).toEqual({
      prompt: { mode: "custom", content: "direct" },
      resources: [
        { kind: "media_control.memory_store", id: "memory", version: 4 },
        { kind: "media_control.skill", id: "skill", version: 2 },
      ],
    });
    expect(
      selectionBody({ mode: "none", content: "old" }, refs).prompt,
    ).toEqual({ mode: "none" });
  });
  it("preserves legacy pinned library choices instead of upgrading to latest", () => {
    expect(
      selectedPrompt({
        agent_config_id: "config",
        version: 10,
        resources: [
          { kind: "media_control.configured_prompt", id: "prompt", version: 2 },
        ],
      }),
    ).toEqual({ mode: "library", resource_id: "prompt", resource_version: 2 });
  });
});
