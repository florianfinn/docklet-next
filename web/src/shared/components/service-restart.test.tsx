import { renderStackDetail } from "@/data/message-code";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { AttentionWidget } from "@/features/dashboard/widgets/AttentionWidget";
import { ServicesCard } from "@/features/stacks/detail/ServicesCard";
import { renderWithApp } from "@/test/render";

import { ServiceRestartDialog } from "./service-restart";
import { serviceRestartStore } from "./service-restart-store";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => {
  serviceRestartStore.set(null);
  simulationStore.reset();
});

describe("single-service restart entrypoints", () => {
  it("the stack page service row asks first and names the service", async () => {
    const adapter = createDemoAdapter();
    const stack = renderStackDetail(await adapter.getStack("paperless"), "de");
    renderWithApp(
      <>
        <ServicesCard stack={stack!} host={undefined} />
        <ServiceRestartDialog />
      </>,
      adapter,
    );
    await userEvent.click(screen.getByRole("button", { name: "broker neu starten" }));
    expect(await screen.findByRole("alertdialog", { name: "Nur Service „broker“ neu starten?" })).toBeTruthy();
  });

  it("the dashboard attention queue asks before restarting an unhealthy service", async () => {
    const item = { id: "u", kind: "unhealthy" as const, tone: "err" as const, weight: 100, stackId: "paperless", service: "webserver", values: { name: "webserver", stack: "Paperless", restarts: 2 } };
    renderWithApp(
      <>
        <AttentionWidget items={[item]} hosts={new Map()} />
        <ServiceRestartDialog />
      </>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Neustart" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Nur Service „webserver“ neu starten?" });
    expect(within(dialog).getByText(/im Stack „Paperless“/) instanceof HTMLElement).toBe(true);
  });
});
