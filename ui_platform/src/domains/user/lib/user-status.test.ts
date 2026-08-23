import { describe, expect, it } from "vitest";
import type { UserRecord } from "@/foundation/types/auth";
import { canSoftDeleteUser, statusTone } from "./user-status";

const user = { id: "u2", status: "active" } as UserRecord;

describe("user status decisions", () => {
  it("only permits deleting another active user", () => {
    expect(canSoftDeleteUser(user, "u1")).toBe(true);
    expect(canSoftDeleteUser(user, "u2")).toBe(false);
    expect(canSoftDeleteUser({ ...user, status: "deleted" }, "u1")).toBe(false);
  });

  it("maps every status to a visible tone", () => {
    expect(statusTone("active")).toContain("emerald");
    expect(statusTone("disabled")).toContain("amber");
    expect(statusTone("deleted")).toContain("slate");
  });
});
