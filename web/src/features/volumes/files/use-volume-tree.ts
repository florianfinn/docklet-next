import { useQueries } from "@tanstack/react-query";

import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { DirListing } from "@/domain/files";
import type { TreeNode } from "@/shared/workspace/file-tree";

/** Volume adapter for the shared file tree: loads every expanded folder and nests the listings. */
export function useVolumeTree(volumeId: string, rootName: string, expanded: ReadonlySet<string>, dirty: ReadonlySet<string>): TreeNode {
  const adapter = useAdapter();
  const dirs = [...expanded];
  const results = useQueries({
    queries: dirs.map((dir) => ({ queryKey: queryKeys.files(volumeId, dir), queryFn: () => adapter.listFiles(volumeId, dir), retry: false })),
  });
  const listings = new Map(dirs.map((dir, i) => [dir, results[i]]));

  const build = (path: string, name: string): TreeNode => {
    const q = listings.get(path);
    const listing = q?.data as DirListing | undefined;
    return {
      id: path,
      name,
      kind: "dir",
      loading: !!q && q.isPending,
      failed: !!q?.isError,
      children: listing?.entries.map((e) => (e.type === "dir" ? build(e.path, e.name) : { id: e.path, name: e.name, kind: "file", dirty: dirty.has(e.path) })),
    };
  };
  return build("/", rootName);
}
