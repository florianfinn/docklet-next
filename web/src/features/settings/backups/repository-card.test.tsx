// Source: docklet-next/issues/398
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that an unavailable host's repository is not read.

import { act, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { Host } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { RepositoryCard } from "./RepositoryCard";

it("R2-i an offline host's repository card reads nothing and shows the host unavailable; online reads once", async () => {
  const adapter = createDemoAdapter();
  const [first] = await adapter.listHosts();
  const host: Host = { ...first!, status: "offline" };
  const read = vi.spyOn(adapter, "getBackupRepository");
  let setHost!: (next: Host) => void;
  function Probe() {
    const [current, set] = useState(host);
    setHost = set;
    return <RepositoryCard host={current} recovery={() => {}} />;
  }
  renderWithApp(<Probe />, adapter);
  expect((await screen.findByRole("status")).textContent).toMatch(/unavailable|nicht verfügbar/i);
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(read).not.toHaveBeenCalled();
  act(() => setHost({ ...host, status: "online" }));
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
});
