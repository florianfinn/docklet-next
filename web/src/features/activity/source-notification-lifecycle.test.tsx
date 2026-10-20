// Source: docklet-next/issues/102
// Revision: 1819a8a7ed4f4b8116c2195c9ba5e3b16a03120a
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original source baseline lifecycle and genuine-notification consumer regressions.

import { act } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, expect, it, vi } from "vitest";
import { queryKeys } from "@/data/query-keys";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { actionDialogStore } from "./action-dialog-store";
import { activityStore } from "./activity-store";
import { presentationStore } from "./run-presentation";
import { sourceFixture, toastCount, waitFor } from "./source-notification.test-support";

afterEach(() => act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); uiStyleStore.reset(); activityStore.reset(); vi.restoreAllMocks(); }));

it.each([false, true])("already complete source (initially empty=%s) keeps genuine recovery outcomes and dismissed history", async (empty) => {
  const f = sourceFixture(); if (!empty) f.history("bad"); await f.discover();
  const view = f.mount();
  try {
    await f.load(); f.put("bad", "dismissed-run"); await f.load();
    await waitFor(() => expect(toastCount()).toBe(1)); await f.dismiss("dismissed-run");
    f.faults.set("bad", "page502"); await f.load();
    expect(f.shows).toHaveBeenCalledTimes(1);
    f.put("bad", "new-during-outage", "failed"); f.faults.delete("bad"); await f.load();
    expect(f.ids()).toEqual(["dismissed-run", "new-during-outage"]);
    await waitFor(() => expect(toastCount()).toBe(1));
    f.state.details["dismissed-run"]!.activity.status = "failed"; await f.load();
    expect(f.ids()).toEqual(["dismissed-run", "new-during-outage", "dismissed-run"]);
    expect(f.health.mock.lastCall?.[0].failures).toEqual([]);
  } finally { view.stop(); f.client.clear(); }
});

it("initially missing source's first complete empty read ends the baseline", async () => {
  const f = sourceFixture(); f.faults.set("bad", "page502"); await f.discover(); const view = f.mount();
  try {
    await f.load(); f.faults.delete("bad"); await f.load(); expect(f.shows).not.toHaveBeenCalled();
    f.put("bad", "after-empty"); await f.load();
    expect(f.ids()).toEqual(["after-empty"]); await waitFor(() => expect(toastCount()).toBe(1));
  } finally { view.stop(); f.client.clear(); }
});

it("valid same-user renewal retains both pending source baseline and completed source notification history", async () => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "partial"); await f.discover(); const view = f.mount();
  try {
    await f.load(); f.put("good", "dismissed-good"); await f.load();
    await waitFor(() => expect(toastCount()).toBe(1)); await f.dismiss("dismissed-good");
    await act(async () => { await f.adapter.changePassword("Example-password1!", "New-example-password2!"); });
    await f.load(); expect(f.ids()).toEqual(["dismissed-good"]);
    f.faults.delete("bad"); await f.load(); expect(f.shows).toHaveBeenCalledTimes(1);
    f.faults.set("good", "page502"); await f.load(); f.put("good", "renewed-genuine", "failed");
    f.faults.delete("good"); await f.load(); expect(f.ids()).toEqual(["dismissed-good", "renewed-genuine"]);
    f.state.details["bad-history-running"]!.activity.status = "failed"; await f.load();
    expect(f.ids()).toEqual(["dismissed-good", "renewed-genuine", "bad-history-running"]);
  } finally { view.stop(); f.client.clear(); }
});

it.each(["replacement", "logout", "auth401", "activity401"])("%s resets initial source history with no prior identity notices", async (boundary) => {
  const f = sourceFixture(); f.history("bad"); await f.discover(); let view = f.mount();
  try {
    await f.load(); f.put("good", "old-owned"); await f.load(); await waitFor(() => expect(toastCount()).toBe(1));
    if (boundary === "logout") await act(async () => { await f.adapter.logout(); });
    if (boundary === "auth401") {
      f.state.authStatus = 401; await act(async () => { await expect(f.discover()).rejects.toMatchObject({ outcome: { status: 401 } }); });
    }
    if (boundary === "activity401") {
      f.faults.set("bad", "page401"); await act(async () => { await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); });
    }
    f.state.authStatus = 200; f.faults.set("bad", "partial"); f.state.lists.good = [];
    f.state.lists.bad = []; f.put("bad", "new-identity-summary", "failed"); f.history("bad");
    if (boundary === "replacement") {
      f.state.user.id = "next-operator"; await act(async () => { await f.discover(); });
    } else {
      view.stop(); f.client.removeQueries({ queryKey: queryKeys.currentUser });
      f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!")); view = f.mount();
    }
    await f.load(); await waitFor(() => expect(toastCount()).toBe(0));
    expect(f.ids()).toEqual(["old-owned"]);
    f.faults.delete("bad"); await f.load();
    expect(f.ids()).toEqual(["old-owned"]); expect(actionDialogStore.get()).toEqual({ waiting: [] });
    expect(f.observed.mock.lastCall?.[0].some((r: { id: string }) => r.id === "old-owned")).toBe(false);
    f.put("bad", "new-identity-genuine"); await f.load();
    expect(f.ids()).toEqual(["old-owned", "new-identity-genuine"]); await waitFor(() => expect(toastCount()).toBe(1));
  } finally { view.stop(); f.client.clear(); }
});

it("synchronous partial snapshot on mount and remount retains recovery seeding", async () => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "partial"); await f.discover();
  // Existing subscribers keep the actual reader snapshot alive across the consumer remount.
  const off = f.adapter.subscribeActions(() => undefined); await f.adapter.listActions(); let view = f.mount();
  try {
    expect(f.shows).not.toHaveBeenCalled(); view.stop(); view = f.mount();
    f.faults.delete("bad"); await f.load(); expect(f.shows).not.toHaveBeenCalled();
    f.put("bad", "after-remount"); await f.load(); expect(f.ids()).toEqual(["after-remount"]);
  } finally { view.stop(); off(); f.client.clear(); }
});

it("global failure while a baseline is pending never completes that source", async () => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "page502"); await f.discover(); const view = f.mount();
  try {
    await f.load();
    await act(async () => { await f.adapter.changePassword("Example-password1!", "New-example-password2!"); });
    f.state.hubStatus = 500;
    await act(async () => { await expect(f.adapter.listActions()).rejects.toBeTruthy(); });
    expect(f.health.mock.lastCall?.[0].failures.some((failure: { hostId?: string }) => failure.hostId === undefined)).toBe(true);
    f.state.hubStatus = 200; f.faults.delete("bad"); await f.load();
    expect(f.shows).not.toHaveBeenCalled(); f.put("good", "after-global"); await f.load(); expect(f.ids()).toEqual(["after-global"]);
  } finally { view.stop(); f.client.clear(); }
});

it("targeted partial-source updates preserve visible transitions without certifying source completeness", async () => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "partial"); await f.discover(); const view = f.mount();
  try {
    await f.load(); f.state.details["bad-history-failed"]!.activity.status = "running";
    const published = f.observed.mock.calls.length;
    await act(async () => { f.sources.at(-1)!.send("bad-history-failed", "bad"); });
    await waitFor(() => expect(f.observed.mock.calls.length).toBeGreaterThan(published));
    expect(f.health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
    expect(f.shows).not.toHaveBeenCalled();
    f.state.details["bad-history-failed"]!.activity.status = "failed"; f.faults.delete("bad"); await f.load();
    expect(f.ids()).toEqual(["bad-history-failed"]); f.put("bad", "targeted-genuine"); await f.load(); expect(f.ids()).toEqual(["bad-history-failed", "targeted-genuine"]);
  } finally { view.stop(); f.client.clear(); }
});

it("authoritative removal retires a pending source without suppressing subsequent healthy notices", async () => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "page502"); await f.discover(); const view = f.mount();
  try {
    await f.load(); f.state.hosts = ["good", "edge"];
    await act(async () => { f.sources.at(-1)!.environments(); await f.adapter.listActions(); });
    expect(f.health.mock.lastCall?.[0].failures).toEqual([]); f.put("good", "after-removal"); await f.load();
    expect(f.ids()).toEqual(["after-removal"]);
  } finally { view.stop(); f.client.clear(); }
});

it("an adapter without the optional health observer keeps normal initial history behavior", async () => {
  const f = sourceFixture(); f.adapter.subscribeActionReadHealth = undefined; f.history("good"); await f.discover();
  const view = f.mount();
  try {
    await f.load(); expect(f.shows).not.toHaveBeenCalled(); f.put("good", "no-health-genuine"); await f.load();
    expect(f.ids()).toEqual(["no-health-genuine"]);
  } finally { view.stop(); f.client.clear(); }
});

it("initial global401 cannot complete a source baseline for a later same-user login", async () => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "page401"); await f.discover(); let view = f.mount();
  try {
    await act(async () => { await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); });
    expect(f.observed.mock.lastCall?.[0]).toEqual([]); expect(f.shows).not.toHaveBeenCalled();
    view.stop(); f.faults.set("bad", "partial");
    f.client.setQueryData(queryKeys.currentUser, await f.adapter.login("operator", "Example-password1!")); view = f.mount();
    await f.load(); f.faults.delete("bad"); await f.load();
    expect(f.shows).not.toHaveBeenCalled(); f.put("bad", "after-initial401"); await f.load();
    expect(f.ids()).toEqual(["after-initial401"]);
  } finally { view.stop(); f.client.clear(); }
});
