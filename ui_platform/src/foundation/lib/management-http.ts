import { appPath } from "@/foundation/lib/app-path";
export class ManagementError extends Error {
  readonly status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ManagementError";
    this.status = status;
  }
}
export async function managementRequest<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    ...options,
    credentials: "include",
    cache: "no-store",
    headers: { "Content-Type": "application/json", ...options.headers },
  });
  if (!response.ok) {
    if (response.status === 401) window.location.href = appPath("/login");
    const data: unknown = await response.json().catch(() => null);
    const message =
      data &&
      typeof data === "object" &&
      "error" in data &&
      typeof data.error === "string"
        ? data.error
        : "request_failed";
    throw new ManagementError(response.status, message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}
export function managementWrite<T>(
  path: string,
  method: string,
  body: unknown,
): Promise<T> {
  return managementRequest<T>(path, { method, body: JSON.stringify(body) });
}
