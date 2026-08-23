import type { UserRecord } from "@/foundation/types/auth";

export function canSoftDeleteUser(
  target: UserRecord,
  currentUserId: string,
): boolean {
  return target.status === "active" && target.id !== currentUserId;
}

export function statusTone(status: UserRecord["status"]): string {
  if (status === "active") return "bg-emerald-50 text-emerald-700";
  if (status === "disabled") return "bg-amber-50 text-amber-700";
  return "bg-slate-100 text-slate-500";
}
