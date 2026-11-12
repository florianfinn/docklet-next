// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { useState } from "react";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { renderWithApp } from "@/test/render";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { ContainerUpdates, ContainerUpdateDialog } from "@/features/containers/ContainerUpdates";
import type { ContainerInfo } from "@/domain/model";
import { StackUpdatePanel } from "./update-panel";
import { StackUpdateDialogController } from "./stack-update-dialog";

export const container: ContainerInfo = { id: "direct~example-id", hostId: "direct", name: "example", origin: "standalone", image: { repository: "example", tag: "1.0.0" }, state: "running", health: "none", ports: [], networks: [], mounts: [], env: {}, restartPolicy: "no", memLimitMb: 0, update: { state: "rebuild", current: { repository: "example", tag: "1.0.0" }, latest: { repository: "example", tag: "1.0.0" }, registry: "docker.io", checkedAt: "2026-11-12T12:00:00Z" } };
function ContainerFlow() {
  const [selection, setSelection] = useState<{ restore: boolean; autoRestore?: boolean }>();
  return <><ContainerUpdates c={container} select={(restore, autoRestore) => setSelection({ restore, autoRestore })} />{selection && <ContainerUpdateDialog container={container} {...selection} close={() => setSelection(undefined)} />}</>;
}
function StackFlow({ service = false }: { service?: boolean }) {
  const cmd = useStackCommands();
  return <><StackUpdatePanel services={[{ name: "app", update: container.update }]} actions={{ onUpdate: () => void cmd.installUpdates("direct~project", service ? ["app"] : undefined), onUpdateChecked: () => void cmd.installUpdates("direct~project", service ? ["app"] : undefined, true) }} /><StackUpdateDialogController /></>;
}
afterEach(() => setAppLocale("de"));
for (const locale of ["en-US", "de"] as const) for (const kind of ["container", "stack", "service"] as const) {
  it.each([false, true])(`W-M ${locale} ${kind} keyboard checked=%s sends the exact update body`, async (checked) => {
    setAppLocale(locale); const f = updateFixture(); await f.adapter.getCurrentUser();
    renderWithApp(kind === "container" ? <ContainerFlow /> : <StackFlow service={kind === "service"} />, f.adapter);
    const t = catalogs[locale].common.stackUpdates;
    const label = kind === "container" ? catalogs[locale].common.updates.actionUpdateContainer : catalogs[locale].common.updates.actionUpdateStack;
    if (checked) {
      const menu = await screen.findByRole("button", { name: t.updateOptions });
      await waitFor(() => expect(menu.hasAttribute("disabled")).toBe(false));
      menu.focus(); await userEvent.keyboard("{Enter}");
      const item = await screen.findByRole("menuitem", { name: t.updateWithCheck });
      expect(item.getAttribute("aria-describedby") !== null).toBe(true);
      expect(document.body.textContent).toContain(t.updateWithCheckHelp);
      item.focus(); await userEvent.keyboard("{Enter}");
    } else {
      const button = await screen.findByRole("button", { name: label });
      await waitFor(() => expect(button.hasAttribute("disabled")).toBe(false)); await userEvent.click(button);
    }
    const dialog = await screen.findByRole("dialog");
    const submit = within(dialog).getByRole("button", { name: kind === "container" ? label : t.update });
    await waitFor(() => expect(submit.hasAttribute("disabled")).toBe(false)); await userEvent.click(submit);
    await waitFor(() => expect(f.writes().filter((w) => w.path.endsWith("/update"))).toHaveLength(1));
    const body = f.writes().find((w) => w.path.endsWith("/update"))!.init.body;
    expect(body === undefined ? undefined : JSON.parse(String(body))).toEqual(kind === "container" ? checked ? { autoRestore: true } : undefined : { expectedRevision: f.state.baseline.savedRevision, ...(kind === "service" ? { services: ["app"] } : {}), ...(checked ? { autoRestore: true } : {}) });
  });
}
