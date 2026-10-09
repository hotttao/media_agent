/** getRandomValues works on LAN HTTP too; randomUUID is secure-context only. */
export function requestKey(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(20));
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}
