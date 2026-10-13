import type { FileSeed } from "./volume-files";

const MB = 1024 * 1024;

export interface UnusedVolumeSeed {
  name: string;
  hostId: string;
  ageDays: number;
  labels: Record<string, string>;
  files: FileSeed[];
}

/** Leftover Docker volumes without a container, for the prune preview; one sits on the disconnected host. */
export const UNUSED_VOLUMES: UnusedVolumeSeed[] = [
  {
    name: "plex_old_config",
    hostId: "atlas",
    ageDays: 210,
    labels: {},
    files: [{ path: "/Preferences.xml", content: '<?xml version="1.0"?>\n<Preferences FriendlyName="Beispiel" />\n' }, { path: "/Cache/PhotoTranscoder.bin", size: 1.2 * 1024 * MB }],
  },
  {
    name: "paperless_export_tmp",
    hostId: "atlas",
    ageDays: 45,
    labels: { "com.docker.compose.project": "paperless-alt" },
    files: [{ path: "/export/manifest.json", content: '{ "documents": 0, "synthetic": true }\n' }, { path: "/export/archive.zip", size: 340 * MB }],
  },
  {
    name: "4f1c2e9a7b3d8f60a5e2c1b9d7f3a6e8c0b4d2f9e7a1c5b3d8f6e2a9c7b1d3f5",
    hostId: "boreal",
    ageDays: 90,
    labels: {},
    files: [{ path: "/data.bin", size: 76 * MB }],
  },
  {
    name: "nextcloud_previews_old",
    hostId: "boreal",
    ageDays: 120,
    labels: {},
    files: [{ path: "/appdata/preview/cache.bin", size: 2.1 * 1024 * MB }],
  },
  {
    name: "wiki_uploads_old",
    hostId: "cedar",
    ageDays: 300,
    labels: {},
    files: [{ path: "/uploads/archiv.tar", size: 512 * MB }],
  },
];
