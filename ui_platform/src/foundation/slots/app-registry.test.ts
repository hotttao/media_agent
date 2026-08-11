import { describe, expect, it } from "vitest";
import { createAppRegistry } from "./app-registry";

const Component = () => null;

describe("createAppRegistry", () => {
  it("rejects duplicate app ids", () => {
    expect(() =>
      createAppRegistry([
        { id: "chat", label: "Chat", Component },
        { id: "chat", label: "Another chat", Component },
      ]),
    ).toThrow("Duplicate app id: chat");
  });
});
