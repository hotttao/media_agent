import {
  render,
  screen,
  waitFor,
  cleanup,
  fireEvent,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getCapability, listCapabilities } from "../lib/api";
import type { Capability } from "../types";
import { CapabilitySelector } from "./capability-selector";

vi.mock("../lib/api", () => ({
  getCapability: vi.fn(),
  listCapabilities: vi.fn(),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
const contract: Capability = {
  key: "jimeng.i2i.multi_image_prompt",
  version: 2,
  provider: "jimeng",
  generation_method: "i2i",
  generation_type: "multi_image_prompt",
  execution_target: "personal_worker",
  output_media_type: "IMAGE",
  inputs: [
    {
      name: "images",
      type: "resource_ref_list",
      media_type: "IMAGE",
      min_items: 1,
    },
    { name: "prompt", type: "string" },
  ],
};
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
function mount(key = contract.key, version = contract.version) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ready = vi.fn(),
    select = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <CapabilitySelector
        capabilityKey={key}
        version={version}
        onReady={ready}
        onSelect={select}
      />
    </QueryClientProvider>,
  );
  return { client, ready, select };
}
describe("published capability selector", () => {
  it("reads an exact version and displays contract without routing internals", async () => {
    vi.mocked(listCapabilities).mockResolvedValue([contract]);
    vi.mocked(getCapability).mockResolvedValue(contract);
    const { client, ready, select } = mount();
    await screen.findByText(
      /capability.executionTarget.*capability.personal_worker/,
      {},
      { timeout: 4000 },
    );
    await waitFor(() => expect(ready).toHaveBeenLastCalledWith(true));
    expect(getCapability).toHaveBeenCalledWith(contract.key, 2);
    fireEvent.change(screen.getByRole("combobox"), {
      target: { value: `${contract.key}@2` },
    });
    expect(select).toHaveBeenCalledWith(contract);
    expect(screen.queryByText("conductor_task_def")).toBeNull();
    client.clear();
  });
  it("does not validate a pinned version using the latest catalog entry", async () => {
    vi.mocked(listCapabilities).mockResolvedValue([contract]);
    vi.mocked(getCapability).mockRejectedValue(new Error("version missing"));
    const { client, ready } = mount(contract.key, 1);
    await screen.findByText("version missing");
    expect(ready).not.toHaveBeenCalledWith(true);
    client.clear();
  });
  it("does not auto-select the first capability or authorize an empty selection", async () => {
    vi.mocked(listCapabilities).mockResolvedValue([contract]);
    const { client, ready, select } = mount("", 1);
    await screen.findByRole("option", { name: /jimeng/ }, { timeout: 4000 });
    expect(select).not.toHaveBeenCalled();
    expect(getCapability).not.toHaveBeenCalled();
    expect(ready).toHaveBeenLastCalledWith(false);
    client.clear();
  });
});
