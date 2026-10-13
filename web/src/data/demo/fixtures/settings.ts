import { HISTORY_LIMITS } from "@/domain/activity-history";
import type { HubSettings } from "@/domain/settings";

import { daysAgo } from "./time";

/** Synthetic hub settings; example domains and RFC 5737 addresses only. */
export const SETTINGS: HubSettings = {
  setupCompleted: true,
  general: { hubName: "Docklet", hubAddress: "192.0.2.10:8080", domainUrl: "https://docklet.example.com", linkBase: "auto", timeZone: "Europe/Berlin" },
  auth: {
    mode: "local",
    local: { username: "admin", passwordSet: true },
    proxy: { userHeader: "Remote-User", emailHeader: "Remote-Email", groupsHeader: "Remote-Groups", trustedProxies: "192.0.2.10/32", logoutUrl: "" },
  },
  users: [
    { id: "u-admin", name: "Alex Beispiel", email: "alex@example.com", access: "full", scope: "all", admin: true },
    { id: "u-kim", name: "Kim Muster", email: "kim@example.com", access: "operate", scope: ["media", "craft"] },
  ],
  hostDefaults: { projectDir: "/home/docker", installDir: "/home/docker/docklet-agent", connection: "direct", puid: "", pgid: "", detectSocketGroup: true },
  updates: { interval: "12h", notify: true },
  backups: {
    defaultRetention: { maxCount: 7, maxSizeGb: null },
    targets: [
      { id: "t-local-atlas", name: "Atlas · Backup-Share", kind: "local", hostId: "atlas", path: "/mnt/user/backups/docklet", lastTest: { ok: true, at: daysAgo(1), message: "Schreibbar, 3,1 TB frei" } },
      { id: "t-smb-nas", name: "NAS (SMB)", kind: "smb", server: "nas.example.com", share: "backups", path: "/docklet", username: "docklet-backup", secretSet: true, options: "vers=3.1.1", lastTest: { ok: true, at: daysAgo(2), message: "Anmeldung erfolgreich, 820 GB frei" } },
      { id: "t-nfs-archive", name: "Archiv (NFS)", kind: "nfs", server: "archive.example.com", path: "/export/docklet", options: "nfsvers=4.1", lastTest: { ok: false, at: daysAgo(1), message: "Zeitüberschreitung beim Mounten" } },
      { id: "t-mount-boreal", name: "Boreal · Zusatzplatte", kind: "mount", hostId: "boreal", path: "/mnt/backup-disk/docklet" },
    ],
  },
  notifications: [
    { id: "n-mail", kind: "email", name: "Admin-Mail", target: "alex@example.com", secretSet: true, enabled: true, events: ["deployFailed", "hostOffline", "backupFailed", "critical"] },
    { id: "n-ntfy", kind: "ntfy", name: "Handy (ntfy)", target: "https://ntfy.example.com/docklet-demo", secretSet: false, enabled: true, events: ["updates", "deployFailed", "unhealthy"] },
  ],
  security: { scanEnabled: true, schedule: { mode: "daily", time: "03:00" } },
  autoHeal: { enabled: false, maxRestarts: 3, windowMinutes: 60, cooldownMinutes: 10, excludedStacks: ["craft"] },
  activity: { maxEvents: HISTORY_LIMITS.events.default, maxFinishedActions: HISTORY_LIMITS.actions.default, keepActionOutput: true, actionPresentation: "dialog" },
};
