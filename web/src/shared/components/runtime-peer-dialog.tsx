// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

import { useEffect, useState } from "react";
import { useTranslations } from "use-intl";
import { registerRuntimePeerConfirmation, type RuntimePeerConfirmation } from "@/data/runtime-peer-confirmation";
import { runtimeReasonKey } from "@/data/runtime-blocker";
import { useAdapter } from "@/data/use-adapter";
import { useHosts } from "@/data/queries";
import { identityLifetime } from "@/data/identity-lifetime";
import { useLocation } from "react-router";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/shared/ui/alert-dialog";

export function RuntimePeerDialog() {
 const t = useTranslations("common.runtimePeers");
 const adapter = useAdapter(), location = useLocation();
 const hosts = useHosts();
 const [requests, setRequests] = useState<RuntimePeerConfirmation[]>([]);
 useEffect(() => {
  const pending = new Set<RuntimePeerConfirmation>();
  const clear = () => { for (const request of pending) request.settle(false); pending.clear(); setRequests([]); };
  const off = registerRuntimePeerConfirmation((request) => {
   pending.add(request);
   request.signal.addEventListener("abort", () => { pending.delete(request); setRequests((all) => all.filter((item) => item !== request)); }, { once: true });
   setRequests((all) => [...all, request]);
  });
  const offIdentity = identityLifetime(adapter).listen(clear);
  return () => { off(); offIdentity(); clear(); };
 }, [adapter, location.key]);
 const request = requests[0];
 if (!request) return null;
 const { blocker } = request;
 const finish = (confirmed: boolean) => { request.settle(confirmed); setRequests((all) => all.filter((item) => item !== request)); };
 const uncertainty = blocker.confirmable && blocker.uncertainty?.peers ? blocker.uncertainty as NonNullable<typeof blocker.uncertainty> & { peers: NonNullable<NonNullable<typeof blocker.uncertainty>["peers"]> } : undefined;
 return <AlertDialog open onOpenChange={(open) => { if (!open) finish(false); }}>
  <AlertDialogContent data-runtime-peer-dialog>
   <AlertDialogHeader><AlertDialogTitle>{t("title")}</AlertDialogTitle><AlertDialogDescription>{uncertainty ? t("question") : t("updateRequired", { host: hosts.data?.find((host) => host.id === request.host)?.name ?? request.host })}</AlertDialogDescription></AlertDialogHeader>
   {uncertainty && <div className="flex max-h-80 flex-col gap-3 overflow-y-auto"><ul className="flex flex-col gap-2">{uncertainty.peers.map((peer) => <li key={peer.peerId}>{t("peer", { name: peer.name, reason: t(runtimeReasonKey(peer.reason)) })}{peer.file && t("file", { file: peer.file })}</li>)}</ul>{uncertainty.count > uncertainty.peers.length && <p>{t("more", { count: uncertainty.count - uncertainty.peers.length })}</p>}</div>}
   <AlertDialogFooter><AlertDialogCancel onClick={() => finish(false)}>{t("cancel")}</AlertDialogCancel>{uncertainty && <AlertDialogAction onClick={() => finish(true)}>{t("confirm")}</AlertDialogAction>}</AlertDialogFooter>
  </AlertDialogContent>
 </AlertDialog>;
}
