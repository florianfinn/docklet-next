// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { ContainerInspector } from "@/features/containers/ContainerInspector";
import { catalogs } from "@/app/i18n/catalog";
import { resourceFixture } from "@/data/http/resources.test-support";
import { observation } from "@/data/http/update-observation.test-support";
import { mapUpdateInfo } from "@/data/http/update-mapping";
import { StackUpdateIndicator, UpdateIndicator } from "@/shared/components/updates/update-indicator";
import { renderWithApp } from "@/test/render";

it("maps live stack and standalone observations and opens their real details", async () => {
  const f = resourceFixture(), original = f.fetcher.getMockImplementation()!;
  f.state.hostIds = ["first"];
  f.fetcher.mockImplementation(async (input) => {
    const response = await original(input);
    const path = new URL(String(input), "https://hub.example.com").pathname;
    if (!path.endsWith("/containers") && !path.endsWith("/projects")) return response;
    const body = await response.json();
    body.data = body.data.map((row: Record<string, unknown>) => path.endsWith("/containers") ? { ...row, labels: {}, updateInfo: observation("rebuild") } : { ...row, runtimeServices: [{ name: "web", image: "example:1.0.0", status: "running" }], updateInfo: { status: "has_update", hasUpdate: true, imageCount: 1, checkedImageCount: 1, imagesWithUpdates: 1, imagesNotPulled: 0, errorCount: 0, serviceUpdates: { web: { imageRef: "example:1.0.0", updateInfo: observation("version") } } } });
    return new Response(JSON.stringify(body), { headers: response.headers });
  });
  const [stack] = await f.adapter.listStacks(), [container] = await f.adapter.listContainers();
  expect(stack!.services[0]!.update.state).toBe("version");
  expect(container!.origin).toBe("standalone");
  expect(container!.update.state).toBe("rebuild");
  const view = renderWithApp(<StackUpdateIndicator name={stack!.name} services={stack!.services} />, f.adapter);
  await userEvent.click(screen.getByRole("button", { name: /app/ }));
  await screen.findByText(catalogs.de.common.updates.titleMinor);
  expect(screen.getAllByText("1.1.0").length > 0).toBe(true);
  view.unmount();
  renderWithApp(<ContainerInspector container={container} host={undefined} onClose={() => {}} />, f.adapter);
  await screen.findByText(catalogs.de.common.updates.titleRebuild);
  expect(screen.queryByRole("button", { name: catalogs.de.common.updates.actionUpdateContainer })).toBeNull();
  expect(screen.queryByRole("button", { name: catalogs.de.common.updates.actionUpdateStack })).toBeNull();
});

it("renders a typed observation reason through the keyboard gesture without raw diagnostics", async () => {
  const update = mapUpdateInfo({ ...observation("error"), code: "registry.authentication-required" }, { repository: "example", tag: "1.0.0" }, "example:1.0.0", true);
  renderWithApp(<UpdateIndicator name="Example" update={update} />);
  screen.getByRole("button", { name: /Example/ }).focus();
  await userEvent.keyboard("{Enter}");
  const panel = await screen.findByRole("dialog");
  expect(panel.textContent).toContain(catalogs.de.images.errors["registry_authentication-required"]);
  await screen.findByText(catalogs.de.common.updates.stale);
  await waitFor(() => expect(document.body.textContent).not.toContain("private raw diagnostic"));
});
