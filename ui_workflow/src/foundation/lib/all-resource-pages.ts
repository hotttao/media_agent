/** Load authorized metadata pages; callers resolve media only for visible cards. */
export async function allResourcePages<T extends { id: string }>(
  load: (page: number) => Promise<{ items: T[]; total: number }>,
): Promise<T[]> {
  const items = new Map<string, T>();
  for (let number = 1; ; number++) {
    const page = await load(number);
    const before = items.size;
    for (const item of page.items) items.set(item.id, item);
    if (items.size >= page.total) return [...items.values()];
    if (items.size === before || page.items.length === 0)
      throw new Error("Resource pagination did not advance");
  }
}
