// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

import { useTranslations } from "use-intl";
import type { components } from "@/api/generated/schema";

type Certificate = components["schemas"]["ProxyCertificateObservation"];
export function CertificateObservation({ value, outdated = false }: { value: Certificate; outdated?: boolean }) {
  const t = useTranslations("proxyContract.ui");
  // Check booleans explicitly: absent or unknown verification is never valid.
  const verified = value.chainTrusted === true && value.hostnameMatch === true && value.validity === "valid" && !value.error;
  return (
    <section className="grid gap-1 break-all text-xs" aria-label={t("servedCertificate")}>
      <p>{t("certificateObserved", { time: value.observedAt })}</p>
      {outdated && <p role="status">{t("certificateOutdated")}</p>}
      <p>{t(verified ? "certificateVerified" : "certificateUnverified")}</p>
      <p>{t("certificateTrust", { value: t(value.chainTrusted === true ? "certificateYes" : "certificateNo") })}</p>
      <p>{t("certificateHostname", { value: t(value.hostnameMatch === true ? "certificateYes" : "certificateNo") })}</p>
      <p>{t("certificateValidity", { value: t(value.validity === "valid" ? "certificateValid" : value.validity === "expired" ? "certificateExpired" : value.validity === "not-yet-valid" ? "certificateNotYetValid" : "certificateUnknown") })}</p>
      <p>{t("certificatePeer", { subject: value.peer.subject, issuer: value.peer.issuer })}</p>
      <p>{t("certificateDates", { before: value.peer.notBefore, after: value.peer.notAfter })}</p>
      <p>{(value.peer.sans ?? []).join(", ")}</p>
      <p>{value.peer.sha256}</p>
      <p>{t("certificateContext", { container: value.epoch.containerId, epoch: value.epoch.startedAt, host: value.route.host, sni: value.route.sni })}</p>
    </section>
  );
}
