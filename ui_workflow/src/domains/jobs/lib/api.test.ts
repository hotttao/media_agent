import { afterEach, describe, expect, it, vi } from "vitest";
import { getJob, listJobs } from "./api";

afterEach(() => vi.unstubAllGlobals());
describe("job status wire contract", () => {
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
