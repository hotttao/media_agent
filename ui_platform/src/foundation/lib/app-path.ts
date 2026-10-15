// API endpoints remain rooted at /api; only browser navigation uses the UI base.
export function appPath(path: string): string {
  const base = import.meta.env.BASE_URL.replace(/\/$/, "");
  return `${base}${path.startsWith("/") ? path : `/${path}`}`;
}
