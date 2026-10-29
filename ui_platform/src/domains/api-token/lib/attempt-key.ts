// getRandomValues is also available on local-network HTTP origins, unlike randomUUID.
// This identifies an issuance attempt; it is never an access credential.
export function newTokenAttemptKey(): string {
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}
