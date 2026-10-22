import { Database, FileCode2, FolderOpen, KeyRound } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { useVolumes } from "@/data/queries";
import type { ComposeModel } from "@/domain/compose/model";
import { mountRef } from "@/domain/volumes";
import { filesHref } from "@/features/volumes/files/files-href";
import { FileTree, type TreeNode } from "@/shared/workspace/file-tree";

export type WorkspaceFile = "compose" | "env";

const PROJECT_OPEN: ReadonlySet<string> = new Set(["project"]);

interface ComposeFileTreeProps {
  projectName: string;
  projectDir: string;
  hostProjectDir?: string;
  hostId: string;
  /** Null for a draft that was never deployed. */
  stackId: string | null;
  active: WorkspaceFile | null;
  extraFiles?: TreeNode[];
  activeExtra?: string | null;
  onOpenExtra?: (path: string, directory: boolean) => void;
  composeDirty: boolean;
  envDirty: boolean;
  model: ComposeModel | null;
  onOpen: (file: WorkspaceFile) => void;
}

/** Compose adapter for the shared file tree: project files plus mounts linked to their file workspace. */
export function ComposeFileTree({ projectName, projectDir, hostProjectDir, hostId, stackId, active, composeDirty, envDirty, model, onOpen, extraFiles = [], activeExtra, onOpenExtra }: ComposeFileTreeProps) {
  const t = useTranslations("editor.files");
  const volumes = useVolumes();
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(PROJECT_OPEN);
  const known = new Map((volumes.data ?? []).map((v) => [v.id, v]));
  const specs = [...new Map((model?.services ?? []).flatMap((s) => s.mounts.filter((m) => m.editable && m.source && m.target)).map((m) => [m.source, `${m.source}:${m.target}`])).entries()];

  const mounts: TreeNode[] = specs.map(([source, spec]) => {
    const ref = stackId ? mountRef({ hostId, stackId }, spec, hostProjectDir) : undefined;
    const volume = ref ? known.get(ref.id) : undefined;
    const isBind = /^[./~]/.test(source);
    return {
      id: `mount:${source}`,
      name: source,
      kind: "link",
      icon: isBind ? <FolderOpen className="size-4 shrink-0" aria-hidden /> : <Database className="size-4 shrink-0" aria-hidden />,
      href: volume ? filesHref(volume) : undefined,
      hint: volume ? t("mountedOpen") : t("mountedLater"),
    };
  });

  const project: TreeNode = {
    id: "project",
    name: projectName || t("project"),
    kind: "dir",
    children: [
      { id: "compose", name: "compose.yaml", kind: "file", icon: <FileCode2 className="nudge size-4 shrink-0" aria-hidden />, dirty: composeDirty },
      { id: "env", name: ".env", kind: "file", icon: <KeyRound className="nudge size-4 shrink-0" aria-hidden />, dirty: envDirty },
      ...extraFiles,
    ],
  };

  return (
    <FileTree
      label={t("title")}
      sections={[{ id: "project", nodes: [project] }, ...(mounts.length ? [{ id: "mounts", title: t("mounted"), nodes: mounts }] : [])]}
      activeId={activeExtra ? `extra:${activeExtra}` : active}
      expanded={expanded}
      onOpen={(node) => {
        if (node.id === "compose" || node.id === "env") return onOpen(node.id);
        if (node.id.startsWith("extra:")) onOpenExtra?.(node.id.slice(6), node.kind === "dir");
        if (node.kind === "dir") setExpanded((current) => { const next = new Set(current); if (next.has(node.id)) next.delete(node.id); else next.add(node.id); return next; });
      }}
      onToggle={(node, on) => setExpanded((current) => { const next = new Set(current); if (on) next.add(node.id); else next.delete(node.id); return next; })}
      footer={projectDir}
      footerTitle={t("projectDir")}
    />
  );
}
