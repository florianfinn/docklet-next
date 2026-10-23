import { ArrowUp, ChevronRight, Download, FilePlus2, FileText, FolderOpen, FolderPlus, HardDrive, RotateCcw, Save, Upload } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useBlocker, useSearchParams } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useFiles } from "@/data/queries";
import { baseName, parentPath, pathSegments, type FileEntry } from "@/domain/files";
import { volumeLabel, type Volume } from "@/domain/volumes";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { downloadBlob } from "@/shared/lib/tar";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { setPreference, usePreference } from "@/shared/preferences/preferences-store";
import { FileTree, type TreeNode } from "@/shared/workspace/file-tree";
import { AsideToggle, EditorAction, EditorPane, Workspace, WorkspaceFrame } from "@/shared/workspace/workspace";

import { useVolumeCapability } from "../use-volume-capability";
import { useFileCommands } from "./use-file-commands";
import { workspaceErrorKey } from "./workspace-error";
import { FileAside } from "./FileAside";
import { FileBody, FileStatus } from "./FileBody";
import { NameDialog } from "./FileDialogs";
import { FolderView } from "./FolderView";
import { isUnder, movedPath, useOpenFiles, type OpenFile } from "./use-open-files";
import { useVolumeTree } from "./use-volume-tree";

const FOLDER_TAB = "@folder";

type Pending = { kind: "create"; type: "dir" | "file" } | { kind: "rename"; entry: FileEntry } | { kind: "delete"; entry: FileEntry } | { kind: "close"; path: string } | null;

const ancestors = (path: string) => ["/", ...pathSegments(path).map((s) => s.path)];

/**
 * File workspace of a volume, built from the same tree, tabs and
 * editor as the Compose workspace. The folder (?path=) and the active file
 * (?file=) live in the URL; open tabs and unsaved edits live in memory.
 */
export function FileWorkspace({ volume, warning }: { volume: Volume; warning?: string }) {
  const t = useTranslations("volumes.files");
  if (volume.kind === "bind") return <div role="status" className="text-sm text-muted-foreground">{t("bindUnavailable")}</div>;
  return <VolumeFileWorkspace key={volume.id} volume={volume} warning={warning} />;
}

function VolumeFileWorkspace({ volume, warning }: { volume: Volume; warning?: string }) {
  const t = useTranslations("volumes.files");
  const [params, setParams] = useSearchParams();
  const path = params.get("path") ?? "/";
  const fileParam = params.get("file");
  const currentView = useRef(`${path}:${fileParam}`);
  useLayoutEffect(() => { currentView.current = `${path}:${fileParam}`; }, [path, fileParam]);
  const listing = useFiles(volume.id, path);

  const createCapability = useVolumeCapability("createEntry", volume.id);
  const uploadCapability = useVolumeCapability("uploadFile", volume.id);
  const writeCapability = useVolumeCapability("writeFile", volume.id);
  const renameCapability = useVolumeCapability("renameEntry", volume.id);
  const deleteCapability = useVolumeCapability("deleteEntry", volume.id);
  const downloadCapability = useVolumeCapability("downloadFile", volume.id);
  const mounted = useRef(true);
  const writeLockRef = useRef(false);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const open = useOpenFiles(volume.id, fileParam);
  const cmd = useFileCommands(volume.id, listing.data?.revision, open.rebase, writeLockRef);
  const [pending, setPending] = useState<Pending>(null);
  const [saving, setSaving] = useState(false);
  const busy = saving || cmd.busy;
  const aside = usePreference("workspaceAside");
  const [secondary, setSecondary] = useState<string | null>(null);
  const upload = useRef<HTMLInputElement>(null);
  // Last file requested through the URL; its navigation may still be pending.
  const requested = useRef<string | null>(fileParam);
  const singleFile = !!volume.fileBind;
  const siblings = (listing.data?.entries ?? []).map((e) => e.name);

  const [expanded, setExpanded] = useState<Set<string>>(() => new Set([...ancestors(path), ...(fileParam ? ancestors(parentPath(fileParam)) : [])]));
  const [seenPath, setSeenPath] = useState(path);
  if (path !== seenPath) {
    setSeenPath(path);
    if (ancestors(path).some((p) => !expanded.has(p))) setExpanded(new Set([...expanded, ...ancestors(path)]));
  }

  const activeFile = fileParam ? open.files.find((f) => f.path === fileParam) : undefined;
  const activeTab = activeFile ? activeFile.path : FOLDER_TAB;
  const dirtyPaths = new Set(open.files.filter((f) => f.dirty).map((f) => f.path));
  const tree = useVolumeTree(volume.id, volumeLabel(volume), expanded, dirtyPaths);

  const blocker = useBlocker(({ currentLocation, nextLocation }) => open.dirty && (currentLocation.pathname !== nextLocation.pathname || new URLSearchParams(nextLocation.search).get("tab") !== "files"));
  const proceeding = useRef(false);

  const setUrl = (next: { path?: string; file?: string | null }) =>
    setParams((p) => {
      const out = new URLSearchParams(p);
      if (next.path !== undefined) out.set("path", next.path);
      if (next.file === null) out.delete("file");
      else if (next.file !== undefined) out.set("file", next.file);
      return out;
    });

  const goFolder = (dir: string) => setUrl({ path: dir, file: null });
  const openFile = (filePath: string) => {
    open.open(filePath);
    requested.current = filePath;
    setUrl({ file: filePath });
  };
  const openEntry = (entry: FileEntry) => (entry.type === "dir" ? goFolder(entry.path) : openFile(entry.path));

  const onTreeOpen = (node: TreeNode) => {
    if (node.kind === "file") return openFile(node.id);
    const isOpen = expanded.has(node.id);
    if (isOpen && path === node.id && !activeFile) toggle(node, false);
    else {
      if (!isOpen) toggle(node, true);
      goFolder(node.id);
    }
  };
  const toggle = (node: TreeNode, on: boolean) =>
    setExpanded((s) => {
      const next = new Set(s);
      if (on) next.add(node.id);
      else if (node.id !== "/") next.delete(node.id);
      return next;
    });

  const downloadEntry = async (entry: { path: string; name: string; size: number }, buffer?: string) => {
    if (buffer !== undefined) return downloadBlob(entry.name, buffer, "text/plain");
    const blob = await cmd.download(entry.path);
    if (!blob || !mounted.current) return;
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a"); link.href = url; link.download = entry.name; link.click(); URL.revokeObjectURL(url);
  };
  const downloadOpen = (file: OpenFile) => void downloadEntry({ path: file.path, name: baseName(file.path), size: file.size }, file.editable ? file.text : undefined);

  const save = async (file: OpenFile) => {
    if (!file.dirty || file.conflict || !writeCapability.available || writeLockRef.current) return;
    writeLockRef.current = true;
    setSaving(true);
    try {
      if (await open.save(file.path) && mounted.current) toast.success(t("editorSaved", { name: baseName(file.path) }));
    } catch (error) {
      if (mounted.current) toast.error(t(`workspaceErrors.${workspaceErrorKey(error)}`));
    } finally {
      writeLockRef.current = false;
      if (mounted.current) setSaving(false);
    }
  };

  const closeTab = (filePath: string, force = false) => {
    const file = open.files.find((f) => f.path === filePath);
    if (file?.dirty && !force) return setPending({ kind: "close", path: filePath });
    const rest = open.files.filter((f) => f.path !== filePath);
    open.close(filePath);
    if (fileParam === filePath || requested.current === filePath) {
      requested.current = rest.at(-1)?.path ?? null;
      setUrl({ file: requested.current });
    }
  };

  const onUpload = async (list: FileList | null) => {
    if (!list?.length) return;
    const owner = currentView.current;
    let n = 0;
    for (const f of [...list]) {
      if (!mounted.current || owner !== currentView.current) break;
      if (await cmd.upload(volume.id, path, { name: f.name, size: f.size, blob: f })) n++;
      else break;
    }
    if (upload.current) upload.current.value = "";
    if (n && mounted.current) toast.success(t("uploaded", { count: n }));
  };

  const folderName = path === "/" ? t("root") : baseName(path);
  const tabs = [
    { id: FOLDER_TAB, label: folderName, icon: <FolderOpen />, title: path },
    ...open.files.map((f) => ({ id: f.path, label: baseName(f.path), icon: <FileText />, dirty: f.dirty, closable: true, title: f.path })),
  ];

  return (
    <WorkspaceFrame>
      <div className="flex flex-wrap items-center gap-2">
        <nav aria-label={t("breadcrumb")} className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5 text-sm">
          <Button variant="ghost" size="sm" className="h-7 gap-1.5 px-2 font-mono" onClick={() => goFolder("/")} aria-current={path === "/" ? "page" : undefined}>
            <HardDrive className="size-3.5" aria-hidden />
            {t("root")}
          </Button>
          {pathSegments(path).map((seg, i, all) => (
            <span key={seg.path} className="flex items-center gap-0.5">
              <ChevronRight className="size-3.5 text-muted-foreground" aria-hidden />
              <Button variant="ghost" size="sm" className={cn("h-7 max-w-48 px-2 font-mono", i === all.length - 1 && "text-foreground")} onClick={() => goFolder(seg.path)} aria-current={i === all.length - 1 ? "page" : undefined}>
                <span className="truncate">{seg.name}</span>
              </Button>
            </span>
          ))}
        </nav>
        <Button variant="outline" size="sm" disabled={path === "/"} onClick={() => goFolder(parentPath(path))} aria-label={t("up")}>
          <ArrowUp />
        </Button>
        {!singleFile && (
          <>
            <Button variant="outline" size="sm" disabled={!createCapability.available || busy || listing.isPending || listing.isPlaceholderData || listing.isError} title={createCapability.reason} onClick={() => setPending({ kind: "create", type: "dir" })}>
              <FolderPlus data-icon="inline-start" />
              {t("newFolder")}
            </Button>
            <Button variant="outline" size="sm" disabled={!createCapability.available || busy || listing.isPending || listing.isPlaceholderData || listing.isError} title={createCapability.reason} onClick={() => setPending({ kind: "create", type: "file" })}>
              <FilePlus2 data-icon="inline-start" />
              {t("newFile")}
            </Button>
            <Button size="sm" disabled={!uploadCapability.available || busy || listing.isPending || listing.isPlaceholderData || listing.isError} title={uploadCapability.reason} onClick={() => upload.current?.click()}>
              <Upload data-icon="inline-start" />
              {t("upload")}
            </Button>
            <input ref={upload} type="file" multiple className="sr-only" tabIndex={-1} aria-hidden onChange={(e) => void onUpload(e.target.files)} />
          </>
        )}
        <AsideToggle value={aside} onChange={(m) => setPreference("workspaceAside", m)} splitDisabled={singleFile} />
      </div>
      {listing.data?.truncated && <p role="status">{t("truncated")}</p>}
      {warning && <p className="rounded-lg border border-warning/30 bg-warning/8 p-2.5 text-xs text-warning">{warning}</p>}

      <Workspace
        label={t("workspace")}
        asideLabel={t("aside.title")}
        asideKind={aside === "split" && !singleFile ? "split" : "details"}
        aside={
          aside === "off" ? undefined : (
            <FileAside
              readOnly={!writeCapability.available}
              mode={singleFile && aside === "split" ? "details" : aside}
              volume={volume}
              warning={warning}
              path={path}
              active={activeFile}
              files={open.files}
              secondary={secondary}
              onSecondary={setSecondary}
              onEdit={(p, text) => open.edit(p, text)}
              onSave={(f) => void save(f)}
              onDownload={downloadOpen}
            />
          )
        }
        sidebar={<FileTree label={t("tree")} sections={[{ id: "files", nodes: [tree] }]} activeId={activeFile ? activeFile.path : path} expanded={expanded} onOpen={onTreeOpen} onToggle={toggle} footer={volume.source} footerTitle={t("sourcePath")} />}
      >
        <EditorPane
          label={t("openFiles")}
          tabs={tabs}
          activeId={activeTab}
          onSelect={(id) => {
            requested.current = id === FOLDER_TAB ? null : id;
            setUrl({ file: requested.current });
          }}
          onClose={(id) => closeTab(id)}
          bodyClassName={activeFile ? undefined : "bg-card"}
          actions={
            activeFile?.status === "ready" && (
              <>
                {activeFile.editable && <EditorAction label={t("discardChanges")} icon={<RotateCcw />} onClick={() => open.discard(activeFile.path)} disabled={!activeFile.dirty || busy} />}
                <EditorAction label={t("download")} icon={<Download />} disabled={!downloadCapability.available} onClick={() => downloadOpen(activeFile)} />
                {activeFile.editable && (
                  <Button size="sm" className="ml-1 h-7" disabled={!activeFile.dirty || activeFile.conflict || busy || !writeCapability.available} title={writeCapability.reason} onClick={() => void save(activeFile)}>
                    <Save data-icon="inline-start" />
                    {t("save")}
                  </Button>
                )}
              </>
            )
          }
          status={activeFile ? <FileStatus file={activeFile} /> : <span>{t("folderStatus", { count: listing.data?.entries.length ?? 0, path })}</span>}
        >
          {activeFile ? (
            <FileBody file={activeFile} readOnly={!writeCapability.available} onEdit={(text) => open.edit(activeFile.path, text)} onSave={() => void save(activeFile)} onDownload={() => downloadOpen(activeFile)} />
          ) : (
            <FolderView
              listing={listing}
              singleFile={singleFile}
              canRename={renameCapability.available && !busy}
              canDelete={deleteCapability.available && !busy}
              canDownload={downloadCapability.available}
              onOpen={openEntry}
              onDownload={(e) => void downloadEntry(e)}
              onRename={(entry) => setPending({ kind: "rename", entry })}
              onDelete={(entry) => setPending({ kind: "delete", entry })}
            />
          )}
        </EditorPane>
      </Workspace>
      <p id="file-editor-help" className="text-[11px] text-muted-foreground">
        {t("inMemory")}
      </p>

      <NameDialog
        open={pending?.kind === "create"}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.kind === "create" && pending.type === "dir" ? t("createTitleDir") : t("createTitleFile")}
        submitLabel={t("create")}
        siblings={siblings}
        onSubmit={async (name) => {
          if (pending?.kind !== "create") return false;
          const owner = currentView.current;
          const ok = await cmd.createEntry(volume.id, path, name, pending.type);
          if (ok && owner === currentView.current) {
            toast.success(t("created", { name }));
            if (pending.type === "file") openFile(`${path === "/" ? "" : path}/${name}`);
          }
          return !!ok;
        }}
      />
      <NameDialog
        open={pending?.kind === "rename"}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.kind === "rename" ? t("renameTitle", { name: pending.entry.name }) : ""}
        submitLabel={t("rename")}
        initial={pending?.kind === "rename" ? pending.entry.name : ""}
        siblings={siblings}
        onSubmit={async (name) => {
          if (pending?.kind !== "rename") return false;
          const from = pending.entry.path;
          const to = `${parentPath(from) === "/" ? "" : parentPath(from)}/${name}`;
          const owner = currentView.current;
          const ok = await cmd.rename(volume.id, from, name);
          if (ok && owner === currentView.current) {
            open.renamed(from, to);
            setExpanded((s) => new Set([...s].map((p) => movedPath(p, from, to))));
            if (fileParam && isUnder(fileParam, from)) setUrl({ file: movedPath(fileParam, from, to) });
            toast.success(t("renamed", { name }));
          }
          return !!ok;
        }}
      />
      <ConfirmDialog
        open={pending?.kind === "delete"}
        onOpenChange={(o) => !o && setPending(null)}
        title={pending?.kind === "delete" ? t("deleteTitle", { name: pending.entry.name }) : ""}
        description={
          <>
            {pending?.kind === "delete" && pending.entry.type === "dir" ? t("deleteBodyDir") : t("deleteBodyFile")} {t("deleteDemo")}
          </>
        }
        confirmLabel={t("delete")}
        destructive
        onConfirm={async () => {
          if (pending?.kind !== "delete") return;
          const entry = pending.entry;
          const owner = currentView.current;
          if (await cmd.removeEntry(volume.id, entry.path) && owner === currentView.current) {
            open.removed(entry.path);
            setExpanded((s) => new Set([...s].filter((p) => !isUnder(p, entry.path))));
            if (fileParam && isUnder(fileParam, entry.path)) setUrl({ file: null });
            toast.success(t("deleted", { name: entry.name }));
          }
        }}
      />
      <ConfirmDialog
        open={pending?.kind === "close"}
        onOpenChange={(o) => !o && setPending(null)}
        title={t("editorDiscardTitle")}
        description={t("editorDiscardBody", { name: pending?.kind === "close" ? baseName(pending.path) : "" })}
        confirmLabel={t("editorDiscard")}
        destructive
        onConfirm={() => pending?.kind === "close" && closeTab(pending.path, true)}
      />
      <ConfirmDialog
        open={blocker.state === "blocked"}
        onOpenChange={(o) => {
          if (!o && !proceeding.current && blocker.state === "blocked") blocker.reset();
          proceeding.current = false;
        }}
        title={t("leaveTitle")}
        description={t("leaveBody", { names: open.files.filter((f) => f.dirty).map((f) => baseName(f.path)).join(", ") })}
        confirmLabel={t("leaveConfirm")}
        destructive
        onConfirm={() => {
          if (blocker.state !== "blocked") return;
          proceeding.current = true;
          blocker.proceed();
        }}
      />
    </WorkspaceFrame>
  );
}
