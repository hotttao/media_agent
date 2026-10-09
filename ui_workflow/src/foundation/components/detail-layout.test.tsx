import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { formatDetailDate, StructuredDetails } from "./detail-layout";
import { createInstance } from "i18next";
import { I18nextProvider } from "react-i18next";
import { DetailField } from "./detail-layout";

describe("StructuredDetails", () => {
  it("translates empty fields and boolean metadata in the current app language", async () => {
    const i18n = createInstance();
    await i18n.init({
      lng: "en-US",
      resources: {
        "en-US": {
          translation: {
            detailCommon: {
              notFilled: "Not provided",
              yes: "Yes",
              noMetadata: "No additional information",
            },
          },
        },
      },
    });
    render(
      <I18nextProvider i18n={i18n}>
        <DetailField label="Description" />
        <StructuredDetails value={{ enabled: true }} />
        <StructuredDetails value={{}} />
      </I18nextProvider>,
    );
    expect(screen.getByText("Not provided")).toBeTruthy();
    expect(screen.getByText("Yes")).toBeTruthy();
    expect(screen.getByText("No additional information")).toBeTruthy();
  });
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
  it("formats a date in the supplied language", () => {
    expect(formatDetailDate("1704067200000000000", "en-US")).toContain("Jan");
  });
});
