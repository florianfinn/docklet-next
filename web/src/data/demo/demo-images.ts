import { imageIdFor, imageReference, parseImageReference, pruneCandidates, type DockerImage, type ImageConsumer } from "@/domain/images";
import type { ContainerInfo, Host } from "@/domain/model";
import { isSystemWorkload } from "@/domain/system";
import { hasUpdate } from "@/domain/updates";

import type { ImagesApi } from "../api";
import { DemoError, failMessage, wait, type DemoContext } from "./demo-context";
import { createdAt, EXTRA_IMAGES, IMAGE_SIZES_MB } from "./fixtures/images";
import { fakeHex, pullScript } from "./run-output";
import { simulateRead, simulationStore } from "./simulation";

type StoredImage = Omit<DockerImage, "consumers" | "update" | "vulnerabilities">;

const architecture = (host: Host | undefined) => (host?.os.includes("Raspberry") ? "arm64" : "amd64");
const sizeOf = (repository: string, seed: string) => IMAGE_SIZES_MB[repository] ?? 60 + (parseInt(fakeHex(seed, 4), 16) % 840);
const layersOf = (seed: string) => 4 + (parseInt(fakeHex(`${seed}#layers`, 2), 16) % 11);
const versionOf = (tag: string) => (/\d/.test(tag) ? tag.replace(/^v/, "").split("-")[0] : undefined);

/**
 * In-memory image store per host. Images in use come from the containers
 * Docker reports (including Docklet's agent); unused and dangling ones from
 * fixtures and earlier versions left behind after updates. Consumers are
 * computed on every read, so an update or recreate moves them at once.
 */
export function createImagesApi({ db, runner, forcedFailure, demoFailure }: DemoContext): ImagesApi {
  const store = new Map<string, StoredImage>();

  const fromContainer = (c: ContainerInfo): StoredImage => {
    const host = db.host(c.hostId);
    const ref = `${c.image.repository}:${c.image.tag}`;
    return {
      id: imageIdFor(c.hostId, c.image.repository, c.image.tag),
      hostId: c.hostId,
      repository: c.image.repository,
      tag: c.image.tag,
      aliases: [],
      version: c.image.version,
      digest: `sha256:${fakeHex(`${c.hostId}/${ref}`, 64)}`,
      sizeMb: sizeOf(c.image.repository, ref),
      createdAt: c.image.published ?? createdAt(20),
      architecture: architecture(host),
      os: "linux",
      layers: layersOf(ref),
    };
  };

  /** Adds images of containers that appeared since the last read (deploys, updates, recreates). */
  const sync = () => {
    for (const c of db.discoveredContainers()) {
      const id = imageIdFor(c.hostId, c.image.repository, c.image.tag);
      if (!store.has(id)) store.set(id, fromContainer(c));
    }
  };

  for (const seed of EXTRA_IMAGES) {
    const key = seed.tag ?? `none-${fakeHex(`${seed.hostId}${seed.repository}`, 6)}`;
    const id = imageIdFor(seed.hostId, seed.repository, key);
    store.set(id, {
      id,
      hostId: seed.hostId,
      repository: seed.repository,
      tag: seed.tag,
      aliases: [],
      version: seed.version,
      digest: `sha256:${fakeHex(`${seed.hostId}/${seed.repository}/${key}`, 64)}`,
      sizeMb: seed.sizeMb ?? sizeOf(seed.repository, key),
      createdAt: createdAt(seed.createdDays),
      architecture: architecture(db.host(seed.hostId)),
      os: "linux",
      layers: layersOf(key),
    });
  }
  sync();

  const consumersOf = (img: StoredImage, containers: ContainerInfo[]): ContainerInfo[] =>
    img.tag === null ? [] : containers.filter((c) => c.hostId === img.hostId && c.image.repository === img.repository && c.image.tag === img.tag);

  const project = (img: StoredImage, containers: ContainerInfo[]): DockerImage => {
    const users = consumersOf(img, containers);
    const consumers = users.map(
      (c): ImageConsumer => ({ containerId: c.id, name: c.name, hostId: c.hostId, stackId: c.stackId, serviceName: c.serviceName, state: c.state, system: isSystemWorkload(c) || undefined }),
    );
    const update = users.find((c) => !isSystemWorkload(c) && hasUpdate(c.update))?.update ?? users.find((c) => !isSystemWorkload(c))?.update;
    const scanned = img.tag === null ? undefined : db.security.images.find((i) => i.hostId === img.hostId && i.ref === `${img.repository}:${img.tag}`);
    return { ...img, consumers, update, vulnerabilities: scanned?.counts };
  };

  const list = (): DockerImage[] => {
    sync();
    const containers = db.discoveredContainers();
    const hosts = new Set(db.hosts.map((h) => h.id));
    return [...store.values()].filter((i) => hosts.has(i.hostId)).map((i) => project(i, containers));
  };

  const requireConnected = (hostId: string): Host => {
    const host = db.host(hostId);
    if (!host) throw new DemoError("not-found");
    if (host.status !== "online") throw new DemoError("host-offline");
    return host;
  };

  return {
    listImages: () => simulateRead("images", () => (simulationStore.get().empty ? [] : structuredClone(list()))),

    async pullImage({ hostId, reference }) {
      const host = requireConnected(hostId);
      const parsed = parseImageReference(reference);
      if (!parsed) throw new DemoError("invalid-name");
      const ref = `${parsed.repository}:${parsed.tag}`;
      const id = imageIdFor(hostId, parsed.repository, parsed.tag);
      const known = store.has(id);
      const requested = !!demoFailure("pullImage");
      const forced = forcedFailure();
      return { actionId: runner.start({
        kind: "pull",
        title: `Image ziehen · ${ref}`,
        target: `${ref} · ${host.name}`,
        hostId,
        services: [ref],
        steps: [
          { key: "collect", label: "Manifest auflösen", detail: `${parsed.repository.includes(".") ? parsed.repository.split("/")[0] : "Docker Hub"} · Simulation` },
          { key: "pull", label: known ? "Stand vergleichen" : "Layer laden", detail: known ? "Digest unverändert" : `${layersOf(ref)} Layer`, ms: known ? 900 : 2200 },
          { key: "save", label: "Image registrieren", detail: architecture(host) },
        ],
        output: pullScript(ref, known),
        failAt: requested || forced ? "pull" : undefined,
        failMessage: requested ? `Registry meldet: Repository ${parsed.repository} nicht gefunden oder Anmeldung nötig (Simulation)` : failMessage,
        onSuccess: () => {
          const now = new Date().toISOString();
          const prev = store.get(id);
          store.set(id, {
            id,
            hostId,
            repository: parsed.repository,
            tag: parsed.tag,
            aliases: prev?.aliases ?? [],
            version: prev?.version ?? versionOf(parsed.tag),
            digest: prev?.digest ?? `sha256:${fakeHex(`${hostId}/${ref}`, 64)}`,
            sizeMb: prev?.sizeMb ?? sizeOf(parsed.repository, ref),
            createdAt: prev?.createdAt ?? createdAt(1 + (parseInt(fakeHex(ref, 2), 16) % 30)),
            pulledAt: now,
            architecture: architecture(host),
            os: "linux",
            layers: prev?.layers ?? layersOf(ref),
          });
          db.pushEvent({ kind: "update", level: "ok", title: `${ref} auf ${host.name} ${known ? "geprüft – aktuell" : "gezogen"}`, hostId });
          db.changed("images");
        },
        onFailure: () => db.pushEvent({ kind: "update", level: "error", title: `${ref}: Ziehen auf ${host.name} fehlgeschlagen`, hostId }),
      }) };
    },

    async removeImage(id) {
      await wait(300);
      const img = store.get(id);
      if (!img) throw new DemoError("not-found");
      requireConnected(img.hostId);
      if (consumersOf(img, db.discoveredContainers()).length) throw new DemoError("in-use");
      store.delete(id);
      db.pushEvent({ kind: "config", level: "info", title: `Image ${imageReference(img)} auf ${db.host(img.hostId)?.name ?? img.hostId} entfernt`, hostId: img.hostId });
      db.changed("images");
    },

    async pruneImages({ hostIds, mode }) {
      const hosts = hostIds.filter((id) => db.host(id)?.status === "online").map((hostId) => {
        const candidates = pruneCandidates(list().filter((i) => i.hostId === hostId), mode);
        for (const image of candidates) store.delete(image.id);
        return { hostId, removed: candidates.map((i) => i.id), skipped: [], failed: [], spaceReclaimed: candidates.reduce((sum, i) => sum + i.sizeMb * 1024 ** 2, 0) };
      });
      db.changed("images");
      return { hosts };
    },
  };
}
