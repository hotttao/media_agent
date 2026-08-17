import { describe, expect, it } from "vitest";

import { decodeSessionEventFrame } from "@/domains/chat/lib/chat-client";

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
