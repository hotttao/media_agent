import { describe, expect, it } from "vitest";
import { isCompanyTemplate } from "./template-choices";

describe("new company workflow template choices", () => {
  it("allows only the current company and never equates legacy ownership with binding permission", () => {
    expect(isCompanyTemplate("company", "company")).toBe(true);
    expect(isCompanyTemplate("company", "other")).toBe(false);
    expect(isCompanyTemplate("company", "")).toBe(false);
    expect(isCompanyTemplate("company", undefined)).toBe(false);
    expect(isCompanyTemplate("", "")).toBe(false);
  });
});
