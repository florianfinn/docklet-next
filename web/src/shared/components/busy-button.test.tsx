import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { useAdapter } from "@/data/use-adapter";
import { renderWithApp } from "@/test/render";

import { BusyButton } from "./busy-button";
import { useBusyAction } from "./use-busy-action";

beforeEach(() => {
  // Slow enough that a loaded worker still sees the running state before the run settles.
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 1 }));
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

function Probe({ fn }: { fn?: () => Promise<string | undefined> }) {
  const adapter = useAdapter();
  const action = useBusyAction(fn ?? (() => adapter.reconnectHost("atlas")));
  return (
    <>
      <BusyButton busy={action.busy} busyLabel="Wird geprüft …" onClick={() => void action.run()}>
        Verbindung prüfen
      </BusyButton>
      <output data-testid="state">{action.state}</output>
      <output data-testid="error">{action.error ? String(action.error) : ""}</output>
    </>
  );
}

const button = () => screen.getByRole("button", { name: /Verbindung prüfen|Wird geprüft/ });
const busy = () => button().getAttribute("aria-busy") === "true";

describe("busy action", () => {
  it("stays busy until the started action run finishes, ignores repeated clicks and keeps focus", { timeout: 30_000 }, async () => {
    const adapter = createDemoAdapter();
    const spy = vi.spyOn(adapter, "reconnectHost");
    renderWithApp(<Probe />, adapter);
    await userEvent.click(button());
    await waitFor(() => expect(screen.getByTestId("state").textContent).toBe("running"));
    expect([busy(), button().textContent, document.activeElement === button(), button().getAttribute("aria-disabled")]).toEqual([true, "Wird geprüft …", true, "true"]);
    await userEvent.click(button());
    await userEvent.click(button());
    expect(spy).toHaveBeenCalledTimes(1);
    // Busy follows the run, not a timer: it ends exactly when the run settles.
    await waitFor(() => expect(screen.getByTestId("state").textContent).toBe("succeeded"), { timeout: 20_000 });
    expect([busy(), button().textContent]).toEqual([false, "Verbindung prüfen"]);
  });

  it("ends the busy state on a failed call and clears the error on the next run", async () => {
    let fail = true;
    let finish!: () => void;
    function Flaky() {
      const [calls, setCalls] = useState(0);
      return (
        <>
          <Probe
            fn={async () => {
              setCalls((c) => c + 1);
              // Hold the response until the test has observed the pending state.
              await new Promise<void>((resolve) => { finish = resolve; });
              if (fail) throw new Error("hub-offline");
              return undefined;
            }}
          />
          <span data-testid="calls">{calls}</span>
        </>
      );
    }
    renderWithApp(<Flaky />);
    await userEvent.click(button());
    expect(busy()).toBe(true);
    await act(async () => finish());
    await waitFor(() => expect(screen.getByTestId("state").textContent).toBe("failed"));
    expect([busy(), screen.getByTestId("error").textContent]).toEqual([false, "Error: hub-offline"]);
    fail = false;
    await userEvent.click(button());
    expect([busy(), screen.getByTestId("error").textContent]).toEqual([true, ""]);
    await act(async () => finish());
    await waitFor(() => expect(screen.getByTestId("state").textContent).toBe("idle"));
    expect([screen.getByTestId("error").textContent, screen.getByTestId("calls").textContent]).toEqual(["", "2"]);
  });

  it("keeps a real disabled state separate from busy", () => {
    renderWithApp(
      <BusyButton busy={false} disabled>
        Agent neu starten
      </BusyButton>,
    );
    const b = screen.getByRole("button", { name: "Agent neu starten" }) as HTMLButtonElement;
    expect([b.disabled, b.hasAttribute("aria-busy")]).toEqual([true, false]);
  });
});
