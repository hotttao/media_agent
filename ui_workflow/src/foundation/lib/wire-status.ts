/** Accept protobuf numbers, numeric strings and named enums; never guess a state. */
export function wireStatus<T extends string>(
  value: unknown,
  names: readonly T[],
  prefixes: readonly string[] = [],
): T | "unknown" {
  if (
    typeof value === "number" ||
    (typeof value === "string" && /^\d+$/.test(value))
  )
    return names[Number(value)] ?? "unknown";
  if (typeof value !== "string") return "unknown";
  let normalized = value.toLowerCase();
  for (const prefix of prefixes)
    if (normalized.startsWith(prefix)) {
      normalized = normalized.slice(prefix.length);
      break;
    }
  return names.includes(normalized as T) ? (normalized as T) : "unknown";
}
