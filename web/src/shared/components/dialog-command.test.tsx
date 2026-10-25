// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original command receipt lifecycle regression coverage.
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useLayoutEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { renderWithApp } from "@/test/render";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { captureCommandFailure } from "./command-failure";
import { DialogCommandFooter, DialogError } from "./dialog-command";
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

function PendingAttempt({ settle }: { settle: Promise<"success" | "failure"> }) {
  const feedback = useDialogCommand(true, "example-resource");
  const adapter = useAdapter();
  const [completed, setCompleted] = useState(false);
  const submit = async () => {
    const result = await feedback.run(async () => {
      const failure = captureCommandFailure(adapter);
      const outcome = await settle;
      if (outcome === "failure") { failure.error("Translated pending rejection"); return undefined; }
      return true;
    });
    if (result) setCompleted(true);
  };
  return <section {...feedback.bindings}>
    <input aria-label="Pending draft" defaultValue="submitted" />
    <button type="button" onClick={() => void submit()}>Start</button>
    <button type="button">Details</button>
    <output data-testid="completion">{completed ? "completed" : "pending"}</output>
    <DialogError feedback={feedback} />
  </section>;
}
function resultBarrier<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
describe.each(["edit", "click"] as const)("pending attempt after %s", (interaction) => {
  it.each(["success", "failure"] as const)("delivers %s without invalidating the issuing session", async (outcome) => {
    const done = resultBarrier<"success" | "failure">();
    renderWithApp(<PendingAttempt settle={done.promise} />);
    await userEvent.click(screen.getByText("Start"));
    if (interaction === "edit") await userEvent.type(screen.getByLabelText("Pending draft"), " changed");
    else await userEvent.click(screen.getByText("Details"));
    await act(async () => { done.resolve(outcome); await done.promise; });
    if (outcome === "success") expect(screen.getByTestId("completion").textContent).toBe("completed");
    else expect(screen.getByRole("alert").textContent).toBe("Translated pending rejection");
    expect(toast.error).not.toHaveBeenCalled();
  });
});
function ConcurrentAttempts({ first, second, completed }: { first: Promise<string>; second: Promise<string>; completed: (value: string) => void }) {
  const feedback = useDialogCommand();
  const start = async (promise: Promise<string>) => { const value = await feedback.run(() => promise); if (value) completed(value); };
  return <section {...feedback.bindings}><button onClick={() => void start(first)}>First</button><button onClick={() => void start(second)}>Second</button></section>;
}
it("starting another attempt in the same session preserves both pending successes", async () => {
  const first = resultBarrier<string>(), second = resultBarrier<string>(), completed = vi.fn();
  renderWithApp(<ConcurrentAttempts first={first.promise} second={second.promise} completed={completed} />);
  await userEvent.click(screen.getByText("First")); await userEvent.click(screen.getByText("Second"));
  await act(async () => { first.resolve("first"); await first.promise; });
  expect(completed).toHaveBeenCalledWith("first");
  await act(async () => { second.resolve("second"); await second.promise; });
  expect(completed.mock.calls).toEqual([["first"], ["second"]]);
});
function EditableConfirmation({ settle }: { settle: Promise<"success" | "failure"> }) {
  const adapter = useAdapter(); const [open, setOpen] = useState(true);
  return <ConfirmDialog open={open} onOpenChange={setOpen} title="Example confirmation" description="Example" confirmLabel="Confirm pending" onConfirm={async () => {
    const failure = captureCommandFailure(adapter);
    if (await settle === "failure") { failure.error("Specific pending rejection"); return false; }
    return true;
  }}><input aria-label="Confirmation draft" /></ConfirmDialog>;
}
it.each(["success", "failure"] as const)("confirmation keeps its %s completion checkpoint after a pending edit", async (outcome) => {
  const done = resultBarrier<"success" | "failure">();
  renderWithApp(<EditableConfirmation settle={done.promise} />);
  await userEvent.click(screen.getByText("Confirm pending"));
  await userEvent.type(screen.getByLabelText("Confirmation draft"), "edited while pending");
  await act(async () => { done.resolve(outcome); await done.promise; });
  if (outcome === "success") expect(screen.queryByRole("alertdialog") === null).toBe(true);
  else expect(screen.getByRole("alert").textContent).toBe("Specific pending rejection");
  expect(toast.error).not.toHaveBeenCalled();
});
let renderParent: () => void;
function FocusHarness() {
  const [tick, setTick] = useState(0);
  useLayoutEffect(() => { renderParent = () => setTick((value) => value + 1); }, []);
  const feedback = useDialogCommand(); const adapter = useAdapter();
  return <section {...feedback.bindings}><output>{tick}</output><DialogCommandFooter feedback={feedback}>
    <button onClick={() => feedback.run(() => captureCommandFailure(adapter).error("Repeated translated rejection"))}>Reject</button>
    <button>Read details</button>
  </DialogCommandFooter></section>;
}
it("parent renders preserve user focus and scroll after an inline failure", async () => {
  const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
  try {
    renderWithApp(<FocusHarness />);
    await userEvent.click(screen.getByText("Reject"));
    expect(document.activeElement === screen.getByRole("alert")).toBe(true);
    const count = scroll.mock.calls.length;
    expect(count).toBe(1);
    await userEvent.tab(); const focused = document.activeElement;
    expect(focused === screen.getByRole("alert")).toBe(false);
    await act(async () => renderParent());
    expect(document.activeElement === focused).toBe(true);
    expect(scroll.mock.calls.length).toBe(count);
  } finally { scroll.mockRestore(); }
});
it("a second distinct failure focuses and scrolls once again even with the same text", async () => {
  const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
  try {
    renderWithApp(<FocusHarness />);
    await userEvent.click(screen.getByText("Reject")); const firstCount = scroll.mock.calls.length;
    expect(firstCount).toBe(1);
    await userEvent.tab();
    await act(async () => renderParent());
    expect(scroll.mock.calls.length).toBe(firstCount);
    await userEvent.click(screen.getByText("Reject"));
    expect(document.activeElement === screen.getByRole("alert")).toBe(true);
    expect(scroll.mock.calls.length).toBe(firstCount + 1);
  } finally { scroll.mockRestore(); }
});

function DetachedAttempt({ settle, owner, notify }: { settle: Promise<boolean>; owner: string; notify: (value: boolean) => void }) {
  const feedback = useDialogCommand(true, owner), adapter = useAdapter();
  return <><button onClick={() => feedback.run(async () => {
    const failure = captureCommandFailure(adapter);
    const ok = await settle;
    if (!ok) failure.error("Detached rejection");
    return ok;
  }, notify)}>Detached start</button><DialogError feedback={feedback} /></>;
}
it.each([true, false])("reports detached success=%s after the owner changes", async (ok) => {
  const done = resultBarrier<boolean>(), notify = vi.fn();
  function Switching() { const [owner, setOwner] = useState("first"); return <><button onClick={() => setOwner("second")}>Switch owner</button><DetachedAttempt settle={done.promise} owner={owner} notify={notify} /></>; }
  renderWithApp(<Switching />);
  await userEvent.click(screen.getByText("Detached start"));
  await userEvent.click(screen.getByText("Switch owner"));
  await act(async () => { done.resolve(ok); await done.promise; });
  expect(notify).toHaveBeenCalledExactlyOnceWith(ok);
  expect(vi.mocked(toast.error).mock.calls).toEqual(ok ? [] : [["Detached rejection", undefined]]);
  expect(screen.queryByRole("alert") === null).toBe(true);
});
it.each([true, false])("suppresses detached success=%s across identity replacement", async (ok) => {
  const done = resultBarrier<boolean>(), notify = vi.fn(), adapter = createDemoAdapter(); let version = 1;
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => true, listen: () => () => {} });
  const view = renderWithApp(<DetachedAttempt settle={done.promise} owner="first" notify={notify} />, adapter);
  await userEvent.click(screen.getByText("Detached start")); view.unmount(); version++;
  await act(async () => { done.resolve(ok); await done.promise; });
  expect(notify).not.toHaveBeenCalled(); expect(toast.error).not.toHaveBeenCalled();
});
