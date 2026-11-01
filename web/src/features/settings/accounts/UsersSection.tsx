// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original live users table based on the approved AuthSection reference.

import { Pencil, Plus, ShieldCheck, Trash2, UserRound } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslations } from "use-intl";
import type { ManagedUser } from "@/data/api";
import { useCurrentUser, useHosts, useStacks } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import type { AuthMode } from "@/domain/settings";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { UserDialog } from "./UserDialog";
import { AssignmentsDialog } from "./AssignmentsDialog";

export function UsersSection({ mode }: { mode: AuthMode }) {
  const t = useTranslations("settings.liveUsers");
  return <section aria-labelledby="live-users-title" className="mt-5 flex flex-col gap-3 border-t pt-5"><div><h3 id="live-users-title" className="text-sm font-semibold">{t("title")}</h3><p className="text-xs text-muted-foreground">{t("description")}</p></div>{mode === "none" ? <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">{t("noneHint")}</p> : <UsersTable mode={mode} />}</section>;
}

function UsersTable({ mode }: { mode: AuthMode }) {
  const t = useTranslations("settings.liveUsers");
  const adapter = useAdapter();
  const current = useCurrentUser();
  const users = useQuery({ queryKey: ["users"], queryFn: () => adapter.listUsers() });
  const hosts = useHosts();
  const stacks = useStacks();
  const [editing, setEditing] = useState<ManagedUser | "new" | null>(null);
  const [reset, setReset] = useState<ManagedUser | null>(null);
  const [assigning, setAssigning] = useState<ManagedUser | null>(null);
  const [deleting, setDeleting] = useState<ManagedUser | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const admins = (users.data ?? []).filter((u) => u.isAdmin && !u.disabledAt).length;
  const last = (u: ManagedUser) => u.isAdmin && !u.disabledAt && admins <= 1;
  // Disable/delete stay locked for the caller's own row and the last enabled administrator.
  const locked = (u: ManagedUser) => u.id === current.data?.id ? t("ownAccount") : last(u) ? t("lastAdmin") : undefined;
  const saved = () => { void users.refetch(); };
  const toggle = async (u: ManagedUser) => {
    setBusy(true); setError(false);
    try { if (u.disabledAt) await adapter.enableUser(u.id); else await adapter.disableUser(u.id); saved(); }
    catch { setError(true); } finally { setBusy(false); }
  };
  return <>
    <Button size="sm" variant="outline" className="self-end" onClick={() => setEditing("new")}><Plus />{t("add")}</Button>
    {mode === "proxy" && <p className="text-xs text-muted-foreground">{t("proxyHint")}</p>}
    {users.isError ? <QueryError error={users.error} onRetry={saved} /> : !users.data ? <p aria-busy="true">{t("loading")}</p> : <div className="overflow-x-auto rounded-xl border"><Table>
      <TableHeader><TableRow><TableHead>{t("name")}</TableHead><TableHead>{t("state")}</TableHead><TableHead>{t("assignments")}</TableHead><TableHead><span className="sr-only">{t("edit")}</span></TableHead></TableRow></TableHeader>
      <TableBody>{users.data.map((u) => <TableRow key={u.id}>
        <TableCell><span className="flex items-center gap-2 font-medium"><UserRound className="size-3.5" />{u.username}{u.isAdmin && <ToneBadge tone="violet"><ShieldCheck />{t("administrator")}</ToneBadge>}</span></TableCell>
        <TableCell>{t(u.disabledAt ? "disabled" : "enabled")}</TableCell>
        <TableCell className="text-xs">{u.assignments.length ? u.assignments.map((a) => {
          const stack = stacks.data?.find((s) => s.hostId === a.environmentId && (s.id.split("~")[1] ?? s.id) === a.projectId);
          const host = hosts.data?.find((h) => h.id === a.environmentId);
          return <div key={`${a.environmentId}~${a.projectId}`}>{host?.name ?? a.environmentId} · {stack?.name ?? a.projectId} · {t(a.level)}</div>;
        }) : t("empty")}</TableCell>
        <TableCell><div className="flex flex-wrap justify-end gap-1">
          <Button size="icon" variant="ghost" aria-label={`${t("edit")}: ${u.username}`} onClick={() => setEditing(u)}><Pencil /></Button>
          <Button size="sm" variant="ghost" onClick={() => setAssigning(u)}>{t("assign")}</Button>
          <Button size="sm" variant="ghost" disabled={busy || (!u.disabledAt && !!locked(u))} title={locked(u)} onClick={() => void toggle(u)}>{t(u.disabledAt ? "enable" : "disable")}</Button>
          {mode === "local" && <Button size="sm" variant="ghost" onClick={() => setReset(u)}>{t("reset")}</Button>}
          <Button size="icon" variant="ghost" aria-label={`${t("remove")}: ${u.username}`} disabled={!!locked(u)} title={locked(u)} onClick={() => setDeleting(u)}><Trash2 /></Button>
        </div></TableCell>
      </TableRow>)}</TableBody>
    </Table></div>}
    {error && <p role="alert">{t("error")}</p>}
    {editing && <UserDialog key={editing === "new" ? "new" : editing.id} user={editing === "new" ? null : editing} mode={mode} currentId={current.data?.id} lastAdmin={editing !== "new" && last(editing)} onClose={() => setEditing(null)} onSaved={saved} />}
    {reset && <UserDialog key={`reset-${reset.id}`} user={reset} mode={mode} currentId={current.data?.id} lastAdmin={last(reset)} reset onClose={() => setReset(null)} onSaved={saved} />}
    {assigning && <AssignmentsDialog key={assigning.id} user={assigning} onClose={() => setAssigning(null)} onSaved={saved} />}
    {deleting && <ConfirmDialog open onOpenChange={(open) => { if (!open) setDeleting(null); }} title={t("confirmDelete", { name: deleting.username })} description={t("deleteHint")} confirmLabel={t("remove")} destructive onConfirm={async () => { await adapter.deleteUser(deleting.id); saved(); }} />}
  </>;
}
