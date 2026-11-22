// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original real-consumer render proofs for all tailnet contract unions.

import { renderHook, screen } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterOutcomeError } from "@/data/capability";
import { useHostError } from "@/features/hosts/use-host-error";
import { mapHost } from "@/data/http/hosts";
import { de } from "@/data/wire-catalogue-render.test-support";
import { renderWithApp } from "@/test/render";
import { TailnetConnection } from "./TailnetConnection";
import { TailnetChain } from "./TailnetHosts";
import { connectedStatus, peerHost, tailnetFixture } from "./tailnet.test-support";

export function tailnetError(code: string) {
  const wrapper = ({ children }: { children: React.ReactNode }) => <IntlProvider locale="de" messages={catalogs.de}>{children}</IntlProvider>;
  return renderHook(() => useHostError(), { wrapper }).result.current(new AdapterOutcomeError({ kind: "error", code }));
}

export async function tailnetState(field: "connection" | "provisioning" | "peer") {
  const status = connectedStatus();
  if (field === "connection") status.connection = { state: "control-unreachable" };
  if (field === "provisioning") status.provisioning = { state: "rejected", code: "tailnet.credential-rejected" };
  const f = tailnetFixture(status);
  const peer = mapHost({ ...peerHost, status: "offline", tailnet: { ...peerHost.tailnet!, state: "unknown", failingLink: "peer" } });
  const view = renderWithApp(field === "peer" ? <TailnetChain host={peer} /> : <TailnetConnection />, f.adapter);
  await screen.findByText(field === "connection" ? de("tailnetContract.connection.control-unreachable") : field === "provisioning" ? de("tailnetContract.provisioning.rejected") : de("tailnetContract.peer.unknown"));
  return view.container.textContent ?? "";
}
