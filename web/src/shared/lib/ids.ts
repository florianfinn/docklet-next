let seq = 0;

/** Client-side id for draft entries (users, targets, channels) before the hub stores them. */
export function draftId(prefix: string): string {
  seq += 1;
  return `${prefix}-${Date.now().toString(36)}-${seq}`;
}
