// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, afterEach } from "vitest";
import { useQueryClient } from "@tanstack/react-query";
import { CertificateObservation } from "./CertificateObservation";
import { CertificateCheck } from "./CertificateCheck";
import { Providers } from "./providers.test-support";
import { instance, route, hash, changedHash } from "./proxy.test-support";
import type { components } from "@/api/generated/schema";
import { checkCertificate } from "./certificate-api";
import { queryKeys } from "@/data/query-keys";

vi.mock("./certificate-api", () => ({ checkCertificate: vi.fn() }));
type Observation = components["schemas"]["ProxyCertificateObservation"];
const value: Observation = {
  observedAt: "2026-11-20T12:00:00Z", instanceId: instance.id,
  epoch: { containerId: instance.identity.containerId, startedAt: instance.identity.startedAt },
  route: { name: "app", host: "app.example.test", sni: "app.example.test", entryPoint: "websecure" },
  peer: { subject: "CN=app.example.test", issuer: "CN=Example CA", sans: ["app.example.test"], notBefore: "2026-01-01T00:00:00Z", notAfter: "2027-01-01T00:00:00Z", sha256: hash },
  chainTrusted: true, hostnameMatch: true, validity: "valid",
};
function Enabled({children}: {children: React.ReactNode}) {
  const cache = useQueryClient();
  cache.setQueryData(["module-states",0], {proxy:true});
  return children;
}
function Controls() {
  const cache = useQueryClient();
  return <><button onClick={() => cache.setQueryData(["module-states",0],{proxy:false})}>Off</button><button onClick={() => cache.setQueryData(queryKeys.currentUser,{id:"other",isAdmin:false})}>User</button><button onClick={() => cache.setQueryData(queryKeys.currentUser,{id:"other-admin",isAdmin:true})}>Switch admin</button></>;
}
afterEach(() => vi.clearAllMocks());
describe("certificate evidence", () => {
  it.each(["chainTrusted", "hostnameMatch", "validity"] as const)("P9 never presents missing or false %s as valid", key => {
    const view = render(<Providers><CertificateObservation value={{...value,[key]:undefined} as unknown as Observation}/></Providers>);
    expect(screen.queryByText("Certificate verification passed.") !== null).toBe(false);
    expect(screen.queryByText("Certificate verification did not pass.") !== null).toBe(true);
    view.rerender(<Providers><CertificateObservation value={{...value,[key]: key === "validity" ? "expired" : false} as Observation}/></Providers>);
    expect(screen.queryByText("Certificate verification passed.") !== null).toBe(false);
  });
  it("shows independent flags and observed wording", () => {
    render(<Providers><CertificateObservation value={{...value,chainTrusted:false}}/></Providers>);
    expect(screen.queryByText("Chain trusted: No or unknown") !== null).toBe(true);
    expect(screen.queryByText("Hostname matches: Yes") !== null).toBe(true);
    expect(screen.queryByText(`Served certificate observed at ${value.observedAt}.`) !== null).toBe(true);
  });
  it.each(["epoch","route"])("P12 marks cached evidence outdated after changed %s", async change => {
    vi.mocked(checkCertificate).mockResolvedValue(value);
    const currentRoute = {...route,rule:"Host(`app.example.test`)"};
    const view = render(<Providers><Enabled><CertificateCheck instance={instance} route={currentRoute} hash={hash}/></Enabled></Providers>);
    fireEvent.click(screen.getByRole("button",{name:"Check certificate"}));
    await screen.findByText("Certificate verification passed.");
    view.rerender(<Providers><Enabled><CertificateCheck instance={change === "epoch" ? {...instance,identity:{...instance.identity,startedAt:"2026-11-20T13:00:00Z"}} : instance} route={currentRoute} hash={change === "route" ? changedHash : hash}/></Enabled></Providers>);
    await act(async () => {await Promise.resolve();});
    expect(screen.queryByText("Outdated: the container epoch or route changed. Check again.") !== null).toBe(true);
    expect(checkCertificate).toHaveBeenCalledTimes(1);
  });
  it("drops a delayed result on a switch between admins", async () => {
    let resolve!: (value: Observation) => void;
    vi.mocked(checkCertificate).mockImplementation(() => new Promise(done => {resolve=done;}));
    render(<Providers><Enabled><CertificateCheck instance={instance} route={{...route,rule:"Host(`app.example.test`)"}} hash={hash}/><Controls/></Enabled></Providers>);
    fireEvent.click(screen.getByRole("button",{name:"Check certificate"}));
    await waitFor(() => expect(checkCertificate).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole("button",{name:"Switch admin"}));
    await act(async () => {await Promise.resolve();resolve(value);await Promise.resolve();});
    expect(screen.queryByText("Certificate verification passed.") !== null).toBe(false);
  });
  it.each(["Off","User"])("P12 ignores delayed observation after %s", async control => {
    let resolve!: (value: Observation) => void;
    vi.mocked(checkCertificate).mockImplementation(() => new Promise(done => {resolve=done;}));
    render(<Providers><Enabled><CertificateCheck instance={instance} route={{...route,rule:"Host(`app.example.test`)"}} hash={hash}/><Controls/></Enabled></Providers>);
    fireEvent.click(screen.getByRole("button",{name:"Check certificate"}));
    await waitFor(() => expect(checkCertificate).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole("button",{name:control}));
    await waitFor(() => expect(screen.queryByRole("button",{name:"Check certificate"}) !== null).toBe(false));
    await act(async () => {resolve(value);await Promise.resolve();});
    expect(screen.queryByText("Certificate verification passed.") !== null).toBe(false);
    expect(screen.queryByRole("button",{name:"Check certificate"}) !== null).toBe(false);
  });
});
