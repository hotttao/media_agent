import { describe, expect, it } from "vitest";
import {
  base64Bytes,
  base64Text,
  bytesToBase64,
  textToBase64,
} from "./catalog-client";

describe("Skill file transfer", () => {
  it("preserves Unicode text and binary uploads larger than the JavaScript argument limit", () => {
    expect(base64Text(textToBase64("技能正文 🎬\n"))).toBe("技能正文 🎬\n");
    const data = new Uint8Array(300_000).map((_, index) => index % 256);
    const decoded = base64Bytes(bytesToBase64(data));
    expect(decoded.length).toBe(data.length);
    expect(decoded.every((value, index) => value === data[index])).toBe(true);
  });
});
