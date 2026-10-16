import { afterEach, describe, expect, it, vi } from "vitest";

import {
  decodeSessionEventFrame,
  interruptTurn,
  listAgentRequests,
  resolveAgentRequest,
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

describe("Agent Requests", () => {
  it("lists requests and resolves through the Gateway API", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            requests: [
              {
                request_id: "pending-1",
                turn_id: "turn/1",
                request_type: "permission",
                payload_json: JSON.stringify({
                  tool_call: { title: "Read project instructions" },
                  options: [{ option_id: "allow", name: "Allow Once" }],
                }),
                status: "pending",
                created_at: 100,
              },
            ],
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            request_id: "request-1",
            turn_id: "turn/1",
            request_type: "permission",
            payload_json: JSON.stringify({
              options: [{ option_id: "allow", name: "Allow Once" }],
            }),
            response_json: JSON.stringify({ option_id: "allow" }),
            status: "resolved",
            created_at: 100,
            resolved_at: 200,
          }),
          {
            status: 200,
          },
        ),
      );

    await expect(
      listAgentRequests("session/1", "turn/1"),
    ).resolves.toMatchObject([
      {
        request_id: "pending-1",
        payload: { tool_call: { title: "Read project instructions" } },
        response: null,
        session_id: "session/1",
      },
    ]);
    await expect(
      resolveAgentRequest("session/1", "turn/1", "request/1", {
        option_id: "allow",
      }),
    ).resolves.toMatchObject({
      request_id: "request-1",
      status: "resolved",
      payload: { options: [{ option_id: "allow", name: "Allow Once" }] },
      response: { option_id: "allow" },
      session_id: "session/1",
    });
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      "/api/chat/sessions/session%2F1/requests?turn_id=turn%2F1",
    );
    expect(fetchMock.mock.calls[1]?.[1]).toMatchObject({ method: "POST" });
    expect(JSON.parse(String(fetchMock.mock.calls[1]?.[1]?.body))).toEqual({
      response_json: JSON.stringify({ option_id: "allow" }),
    });
  });
});

describe("Session resume", () => {
  it("uses the current execution epoch and caller operation id", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response("{}", { status: 200 }));
    const { resumeSession } = await import("./chat-client");
    await resumeSession("session/1", 3, "operation-1");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/sessions/session%2F1/resume",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({
          expected_epoch: 3,
          operation_id: "operation-1",
        }),
      }),
    );
  });
});
