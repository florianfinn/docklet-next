// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original per-stack levels grouped by host in the approved user dialog.

import { useState } from "react";
import { useTranslations } from "use-intl";
import type { AppAssignment, ManagedUser } from "@/data/api";
import { useHosts, useStacks } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

export function AssignmentsDialog({ user, onClose, onSaved }: { user: ManagedUser; onClose: () => void; onSaved: () => void }) {
  const t = useTranslations("settings.liveUsers");
  const adapter = useAdapter();
  const stacks = useStacks();
  const hosts = useHosts();
  const [assignments, setAssignments] = useState(user.assignments);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const unavailable = stacks.isError || hosts.isError || hosts.data?.some((h) => h.inventory?.stale);
  const setLevel = (host: string, project: string, level: string) => setAssignments((old) => [...old.filter((a) => a.environmentId !== host || a.projectId !== project), ...(level === "restricted" || level === "full" ? [{ environmentId: host, projectId: project, level } as AppAssignment] : [])]);
  const save = async () => {
    setBusy(true); setError(false);
    try {
      const visible = assignments.filter((a) => stacks.data?.some((s) => s.hostId === a.environmentId && (s.id.split("~")[1] ?? s.id) === a.projectId));
      await adapter.setUserAppAssignments(user.id, visible); onSaved(); onClose();
    } catch { setError(true); }
    finally { setBusy(false); }
  };
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose(); }}>
    <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg">
      <DialogHeader><DialogTitle>{t("assignmentTitle", { name: user.username })}</DialogTitle><DialogDescription>{t("description")}</DialogDescription></DialogHeader>
      {unavailable ? <p role="alert">{t("stackLoadError")}</p> : stacks.isPending || hosts.isPending ? <p aria-busy="true">{t("loading")}</p> : <fieldset disabled={busy} className="flex flex-col gap-4">{hosts.data.map((host) => <section key={host.id}><h3 className="mb-2 text-sm font-semibold">{host.name}</h3><div className="flex flex-col gap-2">{stacks.data.filter((s) => s.hostId === host.id).map((s) => {
        const project = s.id.split("~")[1] ?? s.id;
        const value = assignments.find((a) => a.environmentId === host.id && a.projectId === project)?.level ?? "none";
        return <div key={s.id} className="flex items-center justify-between gap-3 rounded-lg border p-2"><Label htmlFor={`level-${s.id}`}>{s.name}</Label><Select value={value} onValueChange={(v) => setLevel(host.id, project, v)}><SelectTrigger id={`level-${s.id}`} className="w-40"><SelectValue /></SelectTrigger><SelectContent>{(["none", "restricted", "full"] as const).map((level) => <SelectItem key={level} value={level}>{t(level)}</SelectItem>)}</SelectContent></Select></div>;
      })}</div></section>)}</fieldset>}
      {assignments.some((a) => a.level === "full") && <p className="text-xs text-warning">{t("fullHint")}</p>}
      {error && <p role="alert">{t("error")}</p>}
      <DialogFooter><Button variant="outline" disabled={busy} onClick={onClose}>{t("cancel")}</Button><Button disabled={busy || !!unavailable || stacks.isPending || hosts.isPending} onClick={() => void save()}>{t("save")}</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}
