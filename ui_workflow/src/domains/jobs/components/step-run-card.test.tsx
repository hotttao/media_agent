import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StepRun } from "../types";
import { StepRunCard } from "./step-run-card";

const actions = vi.hoisted(() => ({ confirm: vi.fn(), regenerate: vi.fn() }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, args?: { number?: number }) =>
      args?.number ? `${key} ${args.number}` : key,
  }),
}));
vi.mock("@/foundation/lib/host-shim", () => ({
  hostShim: {
    useInstanceState: (_id: string, _key: string, initial: () => unknown) =>
      useState(initial),
  },
}));
vi.mock("@/domains/jobs/hooks/use-job-mutations", () => ({
  useConfirmStepRun: () => ({ mutate: actions.confirm, isPending: false }),
  useRegenerateStepRun: () => ({
    mutate: actions.regenerate,
    isPending: false,
  }),
}));
const step = {
  id: "step",
  step_key: "image",
  status: "need_confirm",
  template_version: 1,
  candidates: [
    {
      index: 88,
      url: "https://media.example/result.png?sig=a",
      content: "",
      is_primary: false,
    },
  ],
} as StepRun;
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("candidate approval", () => {
  it("unlocks an identical regenerated result only for this applied receipt", () => {
    actions.regenerate.mockImplementation((_input, callbacks) =>
      callbacks.onSuccess({ ...step, last_review_id: "new-review" }),
    );
    const view = render(
      <StepRunCard jobId="job" instanceId="window" stepRun={step} />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "execution.regenerate" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "execution.sendFeedback" }),
    );
    const disabled = () =>
      (
        screen.getByRole("button", {
          name: "execution.regenerate",
        }) as HTMLButtonElement
      ).disabled;
    expect(disabled()).toBe(true);
    view.rerender(
      <StepRunCard
        jobId="job"
        instanceId="window"
        stepRun={{
          ...step,
          last_review_id: "old-review",
          last_review_status: "applied",
        }}
      />,
    );
    expect(disabled()).toBe(true);
    view.rerender(
      <StepRunCard
        jobId="job"
        instanceId="window"
        stepRun={{
          ...step,
          last_review_id: "new-review",
          last_review_status: "applied",
        }}
      />,
    );
    expect(disabled()).toBe(false);
    expect((screen.getByRole("radio") as HTMLInputElement).checked).toBe(false);
  });

  it("does not reopen acceptance when its receipt is applied but projection has not advanced", () => {
    actions.confirm.mockImplementation((_input, callbacks) =>
      callbacks.onSuccess({ ...step, last_review_id: "accept-review" }),
    );
    const view = render(
      <StepRunCard jobId="job" instanceId="window" stepRun={step} />,
    );
    fireEvent.click(screen.getByRole("radio"));
    fireEvent.click(screen.getByRole("button", { name: "execution.confirm" }));
    view.rerender(
      <StepRunCard
        jobId="job"
        instanceId="window"
        stepRun={{
          ...step,
          last_review_id: "accept-review",
          last_review_status: "applied",
        }}
      />,
    );
    expect(
      (
        screen.getByRole("button", {
          name: "execution.confirm",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });
  it("shows results without approval actions when the parent execution is terminal", () => {
    render(
      <StepRunCard
        jobId="job"
        instanceId="window"
        stepRun={step}
        reviewAllowed={false}
      />,
    );
    expect(
      screen.queryByRole("button", { name: "execution.confirm" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "execution.regenerate" }),
    ).toBeNull();
    expect((screen.getByRole("radio") as HTMLInputElement).disabled).toBe(true);
  });
  it("requires explicit selection and sends array position, then waits for the receipt", () => {
    actions.confirm.mockImplementation((_input, callbacks) =>
      callbacks.onSuccess(),
    );
    render(<StepRunCard jobId="job" instanceId="window" stepRun={step} />);
    const accept = screen.getByRole("button", { name: "execution.confirm" });
    expect((accept as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(
      screen.getByRole("radio", { name: "execution.candidate 1" }),
    );
    fireEvent.click(accept);
    expect(actions.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        selectedIndex: 0,
        idempotencyKey: expect.any(String),
      }),
      expect.any(Object),
    );
    expect(screen.getByRole("status").textContent).toBe(
      "execution.decisionPending",
    );
    expect((accept as HTMLButtonElement).disabled).toBe(true);
    expect(
      (
        screen.getByRole("button", {
          name: "execution.regenerate",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it("retains selection for rotated signatures but requires a new choice for a new result", () => {
    const view = render(
      <StepRunCard jobId="job" instanceId="window" stepRun={step} />,
    );
    fireEvent.click(screen.getByRole("radio"));
    view.rerender(
      <StepRunCard
        jobId="job"
        instanceId="window"
        stepRun={{
          ...step,
          candidates: [
            {
              ...step.candidates[0],
              url: "https://media.example/result.png?sig=b",
            },
          ],
        }}
      />,
    );
    expect((screen.getByRole("radio") as HTMLInputElement).checked).toBe(true);
    view.rerender(
      <StepRunCard
        jobId="job"
        instanceId="window"
        stepRun={{
          ...step,
          candidates: [
            {
              ...step.candidates[0],
              url: "https://media.example/new-result.png",
            },
          ],
        }}
      />,
    );
    expect((screen.getByRole("radio") as HTMLInputElement).checked).toBe(false);
    expect(
      (
        screen.getByRole("button", {
          name: "execution.confirm",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });
});
