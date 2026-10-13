import { ChevronRight, ExternalLink, FileText, Folder, FolderOpen, FolderTree } from "lucide-react";
import type { KeyboardEvent, ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { cn } from "@/shared/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export interface TreeNode {
  id: string;
  name: string;
  /** Links leave the workspace (for example to a mounted volume). */
  kind: "dir" | "file" | "link";
  icon?: ReactNode;
  dirty?: boolean;
  /** Directory children; undefined while not loaded. */
  children?: TreeNode[];
  loading?: boolean;
  failed?: boolean;
  href?: string;
  hint?: string;
}

export interface TreeSection {
  id: string;
  title?: string;
  nodes: TreeNode[];
}

interface FileTreeProps {
  label: string;
  sections: TreeSection[];
  activeId?: string | null;
  expanded?: ReadonlySet<string>;
  /** Row activation: files open, directories are handled by the owner. */
  onOpen: (node: TreeNode) => void;
  /** Arrow right/left on a directory. */
  onToggle?: (node: TreeNode, open: boolean) => void;
  footer?: ReactNode;
  footerTitle?: string;
}

const ROW = "flex h-7 w-full min-w-0 items-center gap-2 rounded-md pr-2 text-left text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring";
const NO_EXPANDED: ReadonlySet<string> = new Set();

/** Workspace sidebar: sections of files, folders and outgoing links with keyboard navigation. */
export function FileTree({ label, sections, activeId, expanded = NO_EXPANDED, onOpen, onToggle, footer, footerTitle }: FileTreeProps) {
  const t = useTranslations("workspace.tree");

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const rows = [...e.currentTarget.querySelectorAll<HTMLElement>("[data-tree-row]")];
    const index = rows.indexOf(document.activeElement as HTMLElement);
    if (index < 0) return;
    const move = { ArrowDown: index + 1, ArrowUp: index - 1, Home: 0, End: rows.length - 1 }[e.key];
    if (move !== undefined) {
      e.preventDefault();
      rows[Math.max(0, Math.min(rows.length - 1, move))]?.focus();
      return;
    }
    const row = rows[index];
    if ((e.key === "ArrowRight" || e.key === "ArrowLeft") && row?.dataset.treeDir !== undefined) {
      const node = findNode(sections, row.dataset.treeRow ?? "");
      if (node) {
        e.preventDefault();
        onToggle?.(node, e.key === "ArrowRight");
      }
    }
  };

  const renderNode = (node: TreeNode, depth: number): ReactNode => {
    const indent = { paddingLeft: `${0.5 + depth * 0.875}rem` };
    const active = activeId === node.id;
    if (node.kind === "link") {
      const body = (
        <>
          {node.icon ?? <ExternalLink className="size-4 shrink-0" aria-hidden />}
          <span className="min-w-0 flex-1 truncate font-mono text-[11px]">{node.name}</span>
        </>
      );
      const el = node.href ? (
        <Link to={node.href} data-tree-row={node.id} style={indent} aria-label={node.hint ? `${node.name} – ${node.hint}` : node.name} className={cn(ROW, "group text-muted-foreground transition-colors hover:bg-muted hover:text-foreground")}>
          {body}
        </Link>
      ) : (
        <span tabIndex={0} data-tree-row={node.id} style={indent} aria-label={node.hint ? `${node.name} – ${node.hint}` : node.name} className={cn(ROW, "cursor-default text-muted-foreground/70")}>
          {body}
        </span>
      );
      return (
        <li key={node.id}>
          {node.hint ? (
            <Tooltip>
              <TooltipTrigger asChild>{el}</TooltipTrigger>
              <TooltipContent side="right">{node.hint}</TooltipContent>
            </Tooltip>
          ) : (
            el
          )}
        </li>
      );
    }
    const isDir = node.kind === "dir";
    const open = isDir && expanded.has(node.id);
    const icon = node.icon ?? (isDir ? open ? <FolderOpen className="size-4 shrink-0 text-warning" aria-hidden /> : <Folder className="size-4 shrink-0 text-warning" aria-hidden /> : <FileText className="nudge size-4 shrink-0" aria-hidden />);
    return (
      <li key={node.id}>
        <button
          type="button"
          data-tree-row={node.id}
          data-tree-dir={isDir ? "" : undefined}
          aria-expanded={isDir ? open : undefined}
          aria-current={active ? "true" : undefined}
          onClick={() => onOpen(node)}
          style={indent}
          className={cn(ROW, "group text-muted-foreground transition-colors hover:bg-muted hover:text-foreground", active && "bg-muted text-foreground", isDir && "text-foreground")}
        >
          {isDir ? <ChevronRight className={cn("size-3.5 shrink-0 transition-transform duration-(--dur-fast)", open && "rotate-90")} aria-hidden /> : <span className="w-3.5 shrink-0" aria-hidden />}
          {icon}
          <span className={cn("min-w-0 flex-1 truncate", isDir ? "font-medium" : "font-mono text-xs")}>{node.name}</span>
          {node.dirty && <i className="size-1.5 shrink-0 rounded-full bg-primary" aria-label={t("dirty")} />}
        </button>
        {open && (
          <ul role="group">
            {node.children === undefined || node.loading ? (
              <TreeNote depth={depth + 1}>{node.failed ? t("failed") : t("loading")}</TreeNote>
            ) : node.failed ? (
              <TreeNote depth={depth + 1}>{t("failed")}</TreeNote>
            ) : node.children.length === 0 ? (
              <TreeNote depth={depth + 1}>{t("empty")}</TreeNote>
            ) : (
              node.children.map((child) => renderNode(child, depth + 1))
            )}
          </ul>
        )}
      </li>
    );
  };

  return (
    <nav aria-label={label} data-slot="file-tree" className="flex min-h-0 flex-col border-b border-border/70 bg-muted/15 md:border-r md:border-b-0">
      <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border/70 px-3 text-[10.5px] font-semibold tracking-[0.08em] text-muted-foreground uppercase">
        <FolderTree className="size-3.5" aria-hidden />
        {label}
      </div>
      <div className="flex max-h-56 min-h-0 flex-1 flex-col gap-0.5 overflow-auto p-1.5 md:max-h-none" onKeyDown={onKeyDown}>
        {sections.map((s, i) => (
          <div key={s.id} className={cn(i > 0 && "mt-3 border-t border-border/70 pt-2")}>
            {s.title && <div className="px-2 pb-1 text-[10.5px] font-semibold tracking-[0.08em] text-muted-foreground uppercase">{s.title}</div>}
            <ul className="flex flex-col gap-0.5">{s.nodes.map((n) => renderNode(n, 0))}</ul>
          </div>
        ))}
      </div>
      {footer && (
        <p className="shrink-0 border-t border-border/70 px-3 py-2 font-mono text-[10.5px] leading-snug break-all text-muted-foreground" title={footerTitle}>
          {footer}
        </p>
      )}
    </nav>
  );
}

function TreeNote({ depth, children }: { depth: number; children: ReactNode }) {
  return (
    <li className="py-1 text-[11px] text-muted-foreground italic" style={{ paddingLeft: `${1.4 + depth * 0.875}rem` }}>
      {children}
    </li>
  );
}

function findNode(sections: TreeSection[], id: string): TreeNode | undefined {
  const walk = (nodes: TreeNode[]): TreeNode | undefined => {
    for (const n of nodes) {
      if (n.id === id) return n;
      const hit = n.children && walk(n.children);
      if (hit) return hit;
    }
    return undefined;
  };
  for (const s of sections) {
    const hit = walk(s.nodes);
    if (hit) return hit;
  }
  return undefined;
}
