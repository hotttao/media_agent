export type BrowserFlow = "login" | "registration" | "logout";

function safePath(path: string): string {
  return path.startsWith("/") && !path.startsWith("//") ? path : "/";
}

export function browserFlowURL(
  flow: BrowserFlow,
  returnPath: string,
  origin = window.location.origin,
): string {
  const returnTo = new URL(safePath(returnPath), origin).toString();
  const path = `/self-service/${flow}/browser`;
  return `${path}?return_to=${encodeURIComponent(returnTo)}`;
}

export async function createBrowserLogoutURL(
  returnPath: string,
  origin = window.location.origin,
  request: typeof fetch = fetch,
): Promise<string> {
  const response = await request(browserFlowURL("logout", returnPath, origin), {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(`Unable to create logout flow: ${response.status}`);
  }
  const body = (await response.json()) as { logout_url?: string };
  const logoutURL = new URL(body.logout_url ?? "", origin);
  if (
    logoutURL.origin !== new URL(origin).origin ||
    logoutURL.pathname !== "/self-service/logout"
  ) {
    throw new Error("Kratos returned an invalid logout URL");
  }
  return logoutURL.toString();
}
