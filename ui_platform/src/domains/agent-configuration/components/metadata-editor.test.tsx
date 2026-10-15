import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  MetadataSelection,
  ResourceChoice,
} from "@/domains/agent-configuration/types";
import { i18n } from "@/foundation/lib/i18n";
import { MetadataEditor } from "./metadata-editor";

const selection: MetadataSelection = {
  agent_config_id: "config",
  version: 4,
  prompt: { mode: "library", resource_id: "prompt", resource_version: 1 },
  resources: [
    { kind: "media_control.configured_prompt", id: "prompt", version: 1 },
    { kind: "media_control.skill", id: "skill", version: 1 },
  ],
};
const choices: ResourceChoice[] = [
  { kind: "prompts", id: "prompt", name: "Writer", version: 2 },
  { kind: "skills", id: "skill", name: "Video skill", version: 3 },
];
beforeEach(async () => {
  await i18n.changeLanguage("en-US");
});
afterEach(() => vi.restoreAllMocks());
describe("Agent configuration confirmation", () => {
  it("does not upgrade a pinned choice when newer library versions arrive", () => {
    const fetch = vi.spyOn(globalThis, "fetch");
    const props = {
      selection,
      choices,
      workspace: "workspace",
      canManage: true,
      saved: vi.fn(),
      openLibrary: vi.fn(),
    };
    const view = render(<MetadataEditor {...props} />);
    view.rerender(
      <MetadataEditor
        {...props}
        choices={[
          ...choices.filter((item) => item.id !== "skill"),
          { kind: "skills", id: "skill", name: "Video skill", version: 6 },
        ]}
      />,
    );
    expect(
      (
        screen.getByLabelText(
          "Fixed version · Video skill",
        ) as HTMLSelectElement
      ).value,
    ).toBe("1");
    expect(
      (screen.getByLabelText("Fixed version · Writer") as HTMLSelectElement)
        .value,
    ).toBe("1");
    expect(
      (
        screen.getByRole("button", {
          name: "Review changes",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(fetch).not.toHaveBeenCalled();
  });
  it("reviews the concrete custom prompt and reuses the idempotency key after a network failure", async () => {
    const fetch = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "temporary" }), { status: 503 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ context_version: 5 }), { status: 200 }),
      );
    const saved = vi.fn().mockResolvedValue(undefined);
    render(
      <MetadataEditor
        selection={selection}
        choices={choices}
        workspace="workspace"
        canManage
        saved={saved}
        openLibrary={vi.fn()}
      />,
    );
    fireEvent.change(screen.getByLabelText("System prompt"), {
      target: { value: "custom" },
    });
    fireEvent.change(screen.getByLabelText("Custom prompt content"), {
      target: { value: "Keep this exact prompt" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    expect(fetch).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Confirm and save" }));
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirm and save" }));
    await waitFor(() => expect(saved).toHaveBeenCalledOnce());
    const first = JSON.parse(String(fetch.mock.calls[0]?.[1]?.body));
    const second = JSON.parse(String(fetch.mock.calls[1]?.[1]?.body));
    expect(first.prompt).toEqual({
      mode: "custom",
      content: "Keep this exact prompt",
    });
    expect(first.resources).toEqual([
      { kind: "media_control.skill", id: "skill", version: 1 },
    ]);
    expect(second.idempotency_key).toBe(first.idempotency_key);
    expect(first.expected_version).toBe(4);
  });
  it("does not offer a save command to a read-only workspace viewer", () => {
    render(
      <MetadataEditor
        selection={selection}
        choices={choices}
        workspace="workspace"
        canManage={false}
        saved={vi.fn()}
        openLibrary={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: "Review changes" })).toBeNull();
    expect(
      (screen.getByLabelText("System prompt") as HTMLSelectElement).disabled ||
        screen.getByLabelText("System prompt").closest("fieldset")?.disabled,
    ).toBe(true);
  });
});
