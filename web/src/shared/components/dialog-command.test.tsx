// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original command receipt lifecycle regression coverage.
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { renderWithApp } from "@/test/render";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { captureCommandFailure } from "./command-failure";
import { DialogError } from "./dialog-command";
import { useDialogCommand } from "./use-dialog-command";
import { ConfirmDialog } from "./confirm-dialog";

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
afterEach(() => { vi.clearAllMocks(); localStorage.clear(); });
function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => { resolve = done; });
  return { promise, resolve };
}
function Harness({ settle }: { settle: Promise<void> }) {
  const [open, setOpen] = useState(true);
  const feedback = useDialogCommand(open, open ? "example-resource" : undefined);
  const adapter = useAdapter();
  const submit = () => feedback.run(async () => {
    const failure = captureCommandFailure(adapter);
    await settle;
    failure.error("Translated rejection");
  });
  return <><button onClick={() => feedback.change(false, setOpen)}>Close</button>{open && <section {...feedback.bindings}><input aria-label="Draft" defaultValue="retained" /><button onClick={submit}>Submit</button><DialogError feedback={feedback} /></section>}</>;
}
it("shows an accessible translated rejection, keeps the draft, and clears on edit", async () => {
  const done = deferred();
  renderWithApp(<Harness settle={done.promise} />);
  await userEvent.click(screen.getByText("Submit"));
  await act(async () => done.resolve());
  expect(screen.getByRole("alert").textContent).toBe("Translated rejection");
  expect(document.activeElement === screen.getByRole("alert")).toBe(true);
  expect((screen.getByLabelText("Draft") as HTMLInputElement).value).toBe("retained");
  expect(toast.error).not.toHaveBeenCalled();
  await userEvent.type(screen.getByLabelText("Draft"), " edited");
  expect(screen.queryByRole("alert") === null).toBe(true);
});
it("delivers a failure after explicit close as a toast", async () => {
  const done = deferred();
  renderWithApp(<Harness settle={done.promise} />);
  await userEvent.click(screen.getByText("Submit"));
  await userEvent.click(screen.getByText("Close"));
  await act(async () => done.resolve());
  expect(toast.error).toHaveBeenCalledWith("Translated rejection", undefined);
});
it("does not replace an edited draft with a stale rejection", async () => {
  const done = deferred();
  renderWithApp(<Harness settle={done.promise} />);
  await userEvent.click(screen.getByText("Submit"));
  await userEvent.type(screen.getByLabelText("Draft"), " changed");
  await act(async () => done.resolve());
  expect(screen.queryByRole("alert") === null).toBe(true);
  expect(toast.error).not.toHaveBeenCalled();
});
function Confirmation() {
  const adapter = useAdapter();
  const [open, setOpen] = useState(true);
  return <ConfirmDialog open={open} onOpenChange={setOpen} title="Remove" description="Example" confirmLabel="Confirm" onConfirm={async () => {
    const failure = captureCommandFailure(adapter);
    await Promise.resolve();
    failure.error("Specific translated rejection");
    // Guarded commands may swallow errors and return void.
  }} />;
}
it("keeps confirmations open with the specific swallowed command failure", async () => {
  renderWithApp(<Confirmation />);
  await userEvent.click(screen.getByText("Confirm"));
  await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("Specific translated rejection"));
  expect(screen.queryByRole("alertdialog") !== null).toBe(true);
  expect(toast.error).not.toHaveBeenCalled();
});

it("suppresses a late closed-dialog rejection after browser identity changes", async () => {
  const done = deferred(); const adapter = createDemoAdapter(); let version = 1;
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => true, listen: () => () => {} });
  renderWithApp(<Harness settle={done.promise} />, adapter);
  await userEvent.click(screen.getByText("Submit"));
  await userEvent.click(screen.getByText("Close"));
  version++;
  await act(async () => done.resolve());
  expect(toast.error).not.toHaveBeenCalled();
});
it("suppresses an unrelated unmount without treating navigation as a deliberate dialog close", async () => {
  const done = deferred();
  const view = renderWithApp(<Harness settle={done.promise} />);
  await userEvent.click(screen.getByText("Submit"));
  view.unmount();
  await act(async () => done.resolve());
  expect(toast.error).not.toHaveBeenCalled();
});
