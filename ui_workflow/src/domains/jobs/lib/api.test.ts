import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getJob,
  listJobs,
  confirmStepRun,
  regenerateStepRun,
  listWorkflows,
} from "./api";

afterEach(() => vi.unstubAllGlobals());
describe("job status wire contract", () => {
  it("reads published summaries without assuming the index includes publication receipts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            items: [
              { id: "ready", status: 2, publication_status: "registered" },
              {
                id: "waiting",
                status: "published",
                publication_status: "pending",
              },
              { id: "unknown", status: 2 },
              {
                id: "draft",
                status: "draft",
                publication_status: "registered",
              },
            ],
          }),
        ),
      ),
    );
    expect((await listWorkflows()).map((item) => item.id)).toEqual([
      "ready",
      "waiting",
      "unknown",
    ]);
  });
  it("keeps review receipts separate from actual step completion and sends the stable key", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "s",
          status: "SR_NEED_CONFIRM",
          last_review_id: "r",
          last_review_status: "pending",
          candidates: [],
        }),
      ),
    );
    vi.stubGlobal("fetch", fetch);
    const receipt = await confirmStepRun("s", 1, "same-intent");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
      selected_index: 1,
      idempotency_key: "same-intent",
    });
    expect(receipt.status).toBe("need_confirm");
    expect(receipt.last_review_status).toBe("pending");
  });
  it("submits feedback as ordinary data and recognizes unknown step states", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ id: "s", status: 99 })));
    vi.stubGlobal("fetch", fetch);
    expect(
      (await regenerateStepRun("s", "{{ordinary feedback}}", "feedback-intent"))
        .status,
    ).toBe("unknown");
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
      feedback: "{{ordinary feedback}}",
      idempotency_key: "feedback-intent",
    });
  });
  it("recognizes imported and paused jobs in numeric and named responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            items: [
              { id: "imported", status: 7 },
              { id: "paused", status: "PAUSED" },
              { id: "future", status: 99 },
            ],
            total: 3,
          }),
          { status: 200 },
        ),
      ),
    );
    expect((await listJobs()).items.map((job) => job.status)).toEqual([
      "awaiting_resume",
      "paused",
      "unknown",
    ]);
  });
  it("accepts protobuf named and numeric-string job status without guessing", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({ job: { id: "j1", status: "2" }, step_runs: [] }),
            { status: 200 },
          ),
        ),
    );
    expect((await getJob("j1")).job.status).toBe("running");
  });
});
