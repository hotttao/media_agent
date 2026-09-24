import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { formatDetailDate, StructuredDetails } from "./detail-layout";

describe("StructuredDetails", () => {
  it("renders nested metadata as readable fields instead of JSON text", () => {
    const { container } = render(
      <StructuredDetails
        value={{
          channel: "douyin",
          enabled: true,
          tags: ["beauty", "summer"],
          seller: { name: "Demo Shop" },
        }}
        labels={{ channel: "渠道" }}
      />,
    );
    expect(screen.getByText("渠道")).toBeTruthy();
    expect(screen.getByText("douyin")).toBeTruthy();
    expect(screen.getByText("beauty")).toBeTruthy();
    expect(screen.getByText("Demo Shop")).toBeTruthy();
    expect(container.querySelector("pre")).toBeNull();
  });
});

describe("formatDetailDate", () => {
  it("formats protobuf nanosecond timestamps", () => {
    expect(formatDetailDate("1704067200000000000")).toContain("2024");
  });
});
