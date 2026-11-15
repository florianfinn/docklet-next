// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/query-keys.ts
// Revision: d849ad059a641143623c20eb945b185a0055c8616f38e032b5cab15c74e5271b
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Connected first-run identity flow for #69. See web/PROVENANCE.md. M17 #335 #336 runtime history sections. M17 runtime history transport wiring. M17 hub service widgets replace the simulated widget configuration. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). M23 #435: bounded Dashboard events window and timeline slots. M23 #435: Dashboard security from active counts (AUTO-6). M26 #462: bounded shell counts and raw inventory ownership.

export const queryKeys = {
  authMode: ["auth-mode"],
  currentUser: ["current-user"],
  setupStatus: ["setup-status"],
  connectedSettings: ["connected-settings"],
  hosts: ["hosts"],
  systemComponents: ["system-components"],
  stacks: ["stacks"],
  stackRoot: ["stack"],
  stack: (id: string) => ["stack", id],
  containers: ["containers"],
  historyRoot: ["history"],
  containerHistory: (hostId: string, name: string, range: string) => ["history", "container", hostId, name, range],
  stackHistory: (id: string, range: string) => ["history", "stack", id, range],
  capacity: ["history", "capacity"],
  serviceWidgets: ["service-widgets"],
  serviceWidget: (kind: string) => ["service-widgets", "data", kind],
  serviceWidgetConfig: (kind: string) => ["service-widgets", "config", kind],
  security: ["security"],
  // Under the security prefix: scans, ignores and settings changes refresh it with the overview.
  activeSecurity: ["security", "active"],
  topology: ["topology"],
  networks: ["network-summaries"],
  events: ["events"],
  // Not under the events prefix: an events invalidation (stream open) must not refetch the Dashboard's window.
  dashboardEvents: ["dashboard-events"],
  actions: ["actions"],
  schedule: ["schedule"],
  volumes: ["volumes"],
  navInventory: ["volumes", "nav-inventory"],
  volumeRoot: ["volume"],
  volume: (id: string) => ["volume", id],
  files: (volumeId: string, path: string) => ["files", volumeId, path],
  fileContent: (volumeId: string, path: string) => ["files", volumeId, "content", path],
  fileRoot: ["files"],
  settings: ["settings"],
  images: ["images"],
} as const;
