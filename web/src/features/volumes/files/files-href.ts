import { baseName } from "@/domain/files";

/** Route into the file workspace of a volume or bind mount; file binds open their only file. */
export function filesHref(volume: { id: string; source: string; fileBind?: boolean }, file?: string): string {
  const target = file ?? (volume.fileBind ? `/${baseName(volume.source)}` : undefined);
  const params = new URLSearchParams({ tab: "files" });
  if (target) params.set("file", target);
  return `/volumes/${volume.id}?${params.toString()}`;
}
