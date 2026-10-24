import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { renderWithApp } from "@/test/render";

import { ConfirmDialog } from "./confirm-dialog";

afterEach(() => localStorage.clear());

function Harness({ onConfirm }: { onConfirm: () => void | Promise<unknown> }) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <span data-testid="state">{open ? "open" : "closed"}</span>
      <ConfirmDialog open={open} onOpenChange={setOpen} title="Host entfernen?" description="Synthetischer Host." confirmLabel="Entfernen" destructive onConfirm={onConfirm} />
    </>
  );
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const confirmButton = () => screen.getByRole("button", { name: "Entfernen" });
const state = () => screen.getByTestId("state").textContent;

describe("ConfirmDialog", () => {
  it("closes at once for a synchronous confirm", async () => {
    const onConfirm = vi.fn();
    renderWithApp(<Harness onConfirm={onConfirm} />);
    await userEvent.click(confirmButton());
    expect([onConfirm.mock.calls.length, state()]).toEqual([1, "closed"]);
  });

  it("stays open and busy until an async confirm settles, ignoring repeats and Escape", async () => {
    const d = deferred<boolean>();
    const onConfirm = vi.fn(() => d.promise);
    renderWithApp(<Harness onConfirm={onConfirm} />);
    await userEvent.click(confirmButton());
    await userEvent.click(confirmButton());
    await userEvent.keyboard("{Escape}");
    expect([onConfirm.mock.calls.length, state(), confirmButton().getAttribute("aria-busy"), (screen.getByRole("button", { name: "Abbrechen" }) as HTMLButtonElement).disabled]).toEqual([1, "open", "true", true]);
    d.resolve(true);
    await waitFor(() => expect(state()).toBe("closed"));
  });

  it("keeps the dialog open with an error after a failed confirm and allows a retry", async () => {
    let attempt = 0;
    const onConfirm = vi.fn(async () => {
      attempt += 1;
      if (attempt === 1) return false;
      if (attempt === 2) throw new Error("Host wird noch verwendet");
      return true;
    });
    renderWithApp(<Harness onConfirm={onConfirm} />);
    await userEvent.click(confirmButton());
    expect((await screen.findByRole("alert")).textContent).toContain("erneut versuchen");
    expect([state(), confirmButton().hasAttribute("aria-busy")]).toEqual(["open", false]);

    await userEvent.click(confirmButton());
    await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("Die Anfrage konnte nicht abgeschlossen werden."));

    await userEvent.click(confirmButton());
    await waitFor(() => expect(state()).toBe("closed"));
    expect(onConfirm.mock.calls.length).toBe(3);
  });
});
