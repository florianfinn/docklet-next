// Source: docklet-next/issues/350
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original get-image metadata, loading and failure presentation proofs. M19 web consumer integration.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { createImageMetadata } from "@/data/http/image-metadata";
import { renderWithApp } from "@/test/render";
import { ImageDetails } from "./ImageDetails";
beforeEach(() => simulationStore.set((state) => ({ ...state, latency: "instant", live: false })));
afterEach(() => simulationStore.reset());
it("get-image maps OS, architecture and layer count without returning the layer list", async () => {
  const fetcher = vi.fn<typeof fetch>(async () => new Response(JSON.stringify({ success: true, data: { architecture: "arm64", os: "linux", rootFs: { layers: ["first", "second"] } } })));
  expect(await createImageMetadata("/api", fetcher).getImageMetadata!("example~sha256:abc")).toEqual({ architecture: "arm64", os: "linux", layers: 2 });
  expect(String(fetcher.mock.calls[0]?.[0])).toBe("/api/environments/example/images/sha256%3Aabc");
});
it.each([false, true])("details show loading then metadata or error (%s)", async (fail) => {
  const adapter = createDemoAdapter(), image = (await adapter.listImages())[0]!, host = (await adapter.listHosts()).find((row) => row.id === image.hostId);
  let resolve!: (value: { architecture: string; os: string; layers: number }) => void, reject!: (reason: Error) => void;
  adapter.getImageMetadata = () => new Promise((yes, no) => { resolve = yes; reject = no; });
  renderWithApp(<ImageDetails id={image.id} image={image} host={host} onClose={() => {}} onPull={() => {}} onRemove={() => {}} />, adapter);
  await waitFor(() => expect(screen.getAllByText("Metadaten werden geladen …").length).toBe(2));
  await act(() => { if (fail) reject(Error("unavailable")); else resolve({ architecture: "arm64", os: "linux", layers: 17 }); });
  if (fail) await waitFor(() => expect(screen.getAllByText("Metadaten nicht verfügbar").length).toBe(2));
  else { expect(await screen.findByText("linux/arm64") !== null).toBe(true); expect(screen.queryByText("17") !== null).toBe(true); }
});
