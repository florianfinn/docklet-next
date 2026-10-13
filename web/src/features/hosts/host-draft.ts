export const HEX = /^#[0-9a-f]{6}$/i;

/** Validation of the host draft; the adapter checks uniqueness. */
export function hostDraftError(name: string, color: string): "nameEmpty" | "nameLong" | "colorInvalid" | null {
  const n = name.trim();
  if (!n) return "nameEmpty";
  if (n.length > 40) return "nameLong";
  if (!HEX.test(color)) return "colorInvalid";
  return null;
}
