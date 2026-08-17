import { afterEach, describe, expect, it, vi } from "vitest";

import {
  decodeSessionEventFrame,
  interruptTurn,
} from "@/domains/chat/lib/chat-client";

afterEach(() => vi.unstubAllGlobals());

describe("decodeSessionEventFrame", () => {
  it("delivers transient message frames without advancing the durable cursor", () => {
    const decoded = decodeSessionEventFrame(
      'event: message\ndata: {"type":"item.delta","delta":{"text":"hi"}}',
      7,
    );

    expect(decoded?.event.type).toBe("item.delta");
    expect(decoded?.cursor).toBe(7);
  });

  it("advances the cursor only for a newer durable event", () => {
    const decoded = decodeSessionEventFrame(
      'id: 8\nevent: message\ndata: {"type":"item.completed"}',
      7,
    );

    expect(decoded?.cursor).toBe(8);
    expect(
      decodeSessionEventFrame(
        'id: 7\nevent: message\ndata: {"type":"item.completed"}',
        7,
      ),
    ).toBeNull();
  });

  it("ignores ready and heartbeat control frames", () => {
    expect(
      decodeSessionEventFrame('event: ready\ndata: {"ok":true}', 7),
    ).toBeNull();
    expect(decodeSessionEventFrame("event: heartbeat\ndata: {}", 7)).toBeNull();
  });
});

describe("interruptTurn", () => {
  it("submits the interrupt command for the active session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({ turn_id: "turn-1", status: "interrupting" }),
        {
          status: 202,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(interruptTurn("session-1")).resolves.toEqual({
      turn_id: "turn-1",
      status: "interrupting",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/chat/sessions/session-1/interrupt",
      expect.objectContaining({ method: "POST", credentials: "include" }),
    );
  });
});
