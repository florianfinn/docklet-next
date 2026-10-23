import { Download, File, FileText, Folder, MoreHorizontal, Pencil, TextCursorInput, Trash2 } from "lucide-react";
import { useTranslations } from "use-intl";

import { workspaceErrorKey } from "./workspace-error";
import type { DirListing, FileEntry } from "@/domain/files";
import { formatBytes, formatDateTime } from "@/domain/format";
import { EmptyState, QueryError } from "@/shared/components/page";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";
import { Skeleton } from "@/shared/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";

interface FolderViewProps {
  listing: { data?: DirListing; isPending: boolean; isError: boolean; isPlaceholderData: boolean; error?: unknown; refetch: () => unknown };
  singleFile: boolean;
  canRename?: boolean;
  canDelete?: boolean;
  canDownload?: boolean;
  onOpen: (entry: FileEntry) => void;
  onDownload: (entry: FileEntry) => void;
  onRename: (entry: FileEntry) => void;
  onDelete: (entry: FileEntry) => void;
}

/** Folder tab of the volume workspace: entries of the current folder with size, date and actions. */
export function FolderView({ listing, singleFile, canRename = true, canDelete = true, canDownload = true, onOpen, onDownload, onRename, onDelete }: FolderViewProps) {
  const t = useTranslations("volumes.files");
  const entries = listing.data?.entries ?? [];
  if (listing.isPending)
    return (
      <div className="flex flex-col gap-1.5 p-3">
        {Array.from({ length: 6 }, (_, i) => (
          <Skeleton key={i} className="h-9 rounded-lg" />
        ))}
      </div>
    );
  if (listing.isError)
    return (
      <div className="p-3">
        <p role="alert">{t(`workspaceErrors.${workspaceErrorKey(listing.error)}`)}</p>
        <QueryError error={listing.error} onRetry={() => void listing.refetch()} compact />
      </div>
    );
  if (entries.length === 0) return <EmptyState icon={<Folder />} title={t("empty")} className="m-3 border-0" />;
  return (
    <div className="h-full overflow-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="pl-4">{t("name")}</TableHead>
            <TableHead className="w-28">{t("size")}</TableHead>
            <TableHead className="hidden w-40 sm:table-cell">{t("modified")}</TableHead>
            <TableHead className="w-12">
              <span className="sr-only">{t("actions")}</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className={cn(listing.isPlaceholderData && "opacity-60")}>
          {entries.map((e) => {
            const Icon = e.type === "dir" ? Folder : e.editable ? FileText : File;
            return (
              <TableRow key={e.path} className="group">
                <TableCell className="py-1.5 pl-4">
                  <button type="button" onClick={() => onOpen(e)} aria-label={t("open", { name: e.name })} className="flex max-w-full items-center gap-2 rounded-sm text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring">
                    <Icon className={cn("size-4 shrink-0", e.type === "dir" ? "text-warning" : "text-muted-foreground")} aria-hidden />
                    <span className="truncate font-mono text-[13px]">{e.name}</span>
                  </button>
                </TableCell>
                <TableCell className="py-1.5 font-mono text-xs text-muted-foreground tabular">{formatBytes(e.size)}</TableCell>
                <TableCell className="hidden py-1.5 text-xs text-muted-foreground sm:table-cell">{formatDateTime(e.modifiedAt)}</TableCell>
                <TableCell className="py-1.5 text-right">
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="size-7" aria-label={t("menu", { name: e.name })}>
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-48">
                      <DropdownMenuGroup>
                        {e.type === "file" && (
                          <DropdownMenuItem onSelect={() => onOpen(e)}>
                            <Pencil /> {e.editable ? t("edit") : t("view")}
                          </DropdownMenuItem>
                        )}
                        {e.type === "file" && (
                          <DropdownMenuItem disabled={!canDownload} onSelect={() => onDownload(e)}>
                            <Download /> {t("download")}
                          </DropdownMenuItem>
                        )}
                        {!singleFile && (
                          <DropdownMenuItem disabled={!canRename || listing.isPlaceholderData} onSelect={() => onRename(e)}>
                            <TextCursorInput /> {t("rename")}
                          </DropdownMenuItem>
                        )}
                      </DropdownMenuGroup>
                      {!singleFile && (
                        <>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" disabled={!canDelete || listing.isPlaceholderData} onSelect={() => onDelete(e)}>
                            <Trash2 /> {t("delete")}
                          </DropdownMenuItem>
                        </>
                      )}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
