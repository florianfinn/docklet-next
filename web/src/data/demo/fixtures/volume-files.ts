/* Synthetic file trees for volumes and bind mounts. Contents are short
   placeholders; binary files carry a size only and cannot be opened. */

export interface FileSeed {
  /** Absolute path inside the volume; a trailing slash marks an empty directory. */
  path: string;
  size?: number;
  content?: string;
  ageDays?: number;
}

const KB = 1024;
const MB = 1024 * KB;
const GB = 1024 * MB;

const t = (path: string, content: string, ageDays = 2): FileSeed => ({ path, content, ageDays });
const b = (path: string, size: number, ageDays = 1): FileSeed => ({ path, size, ageDays });
const d = (path: string): FileSeed => ({ path: path.endsWith("/") ? path : `${path}/` });

const POSTGRES = (version: string, dbSize: number): FileSeed[] => [
  t("/PG_VERSION", `${version}\n`, 40),
  t("/postgresql.conf", "# Synthetic postgresql.conf\nlisten_addresses = '*'\nmax_connections = 100\nshared_buffers = 128MB\n", 40),
  t("/pg_hba.conf", "# TYPE  DATABASE  USER  ADDRESS  METHOD\nhost    all       all   all      scram-sha-256\n", 40),
  b("/base/1/1259", 112 * KB, 6),
  b("/base/16384/2619", dbSize, 0.2),
  b("/base/16384/16397", Math.round(dbSize / 3), 0.2),
  b("/global/pg_control", 8 * KB, 0.1),
  b("/pg_wal/000000010000000000000004", 16 * MB, 0.05),
  d("/pg_stat_tmp"),
];

const REDIS: FileSeed[] = [b("/dump.rdb", 2.4 * MB, 0.1), b("/appendonlydir/appendonly.aof.1.incr.aof", 640 * KB, 0.02), t("/appendonlydir/appendonly.aof.manifest", "file appendonly.aof.1.base.rdb seq 1 type b\n", 3)];

const BY_KEY: Record<string, FileSeed[]> = {
  media_config: [
    t("/system.xml", '<?xml version="1.0"?>\n<ServerConfiguration>\n  <ServerName>Jellyfin (Beispiel)</ServerName>\n  <EnableMetrics>false</EnableMetrics>\n</ServerConfiguration>\n', 5),
    t("/network.xml", '<?xml version="1.0"?>\n<NetworkConfiguration>\n  <PublishedServerUrl>https://media.example.com</PublishedServerUrl>\n</NetworkConfiguration>\n', 12),
    b("/data/library.db", 182 * MB, 0.1),
    b("/data/jellyfin.db", 24 * MB, 0.1),
    t("/log/log_20261009.log", "[INF] Main: Jellyfin version 10.10.3 (synthetic)\n[INF] Startup complete 0:00:04.2\n", 1),
    b("/metadata/library/0a/0a3f/poster.jpg", 420 * KB, 9),
    b("/metadata/library/1c/1c77/poster.jpg", 388 * KB, 9),
    d("/cache"),
  ],
  redis_data: REDIS,
  paperless_data: [b("/celerybeat-schedule.db", 16 * KB, 0.1), t("/migration_lock", "", 30), b("/index/MAIN_WRITELOCK", 0, 2), b("/index/_MAIN_1.toc", 4 * KB, 2), t("/log/paperless.log", "[INFO] [paperless.consumer] Document scan_0001.pdf consumed (synthetic)\n", 1)],
  pg_data: POSTGRES("17", 420 * MB),
  cloud_pg: POSTGRES("17", 640 * MB),
  wiki_pg: POSTGRES("16", 96 * MB),
  ha_config: [
    t("/configuration.yaml", "# Synthetic Home Assistant configuration\ndefault_config:\n\nhomeassistant:\n  name: Beispielhaus\n  time_zone: Europe/Berlin\n\nautomation: !include automations.yaml\n", 3),
    t("/automations.yaml", "- id: '1700000000000'\n  alias: Flurlicht abends\n  trigger:\n    - platform: sun\n      event: sunset\n  action:\n    - service: light.turn_on\n      target:\n        entity_id: light.flur\n", 4),
    t("/secrets.yaml", "token: test-only\n", 20),
    b("/home-assistant_v2.db", 220 * MB, 0.02),
    t("/.storage/core.config_entries", '{\n  "version": 1,\n  "data": { "entries": [] }\n}\n', 6),
    t("/home-assistant.log", "2026-10-10 08:00:01 INFO (MainThread) [homeassistant.core] Starting (synthetic)\n", 0.3),
  ],
  mqtt_data: [b("/mosquitto.db", 340 * KB, 0.1)],
  cloud_data: [
    t("/config/config.php", "<?php\n$CONFIG = array (\n  'trusted_domains' => array ('cloud.example.com'),\n  'dbtype' => 'pgsql',\n);\n", 8),
    t("/data/alex/files/Dokumente/Notizen.md", "# Notizen (synthetisch)\n\n- Backup prüfen\n- Updates einspielen\n", 2),
    b("/data/alex/files/Fotos/Urlaub-001.jpg", 3.8 * MB, 40),
    b("/data/alex/files/Fotos/Urlaub-002.jpg", 4.1 * MB, 40),
    b("/data/nextcloud.log", 2.2 * MB, 0.1),
    d("/apps"),
  ],
  craft_world: [
    t("/server.properties", "# Synthetic server.properties\nmotd=Docklet Demo-Welt\nmax-players=10\ndifficulty=normal\nenable-rcon=true\n", 7),
    t("/ops.json", "[]\n", 30),
    t("/whitelist.json", '[\n  { "name": "BeispielSpieler" }\n]\n', 30),
    b("/world/level.dat", 6 * KB, 0.01),
    b("/world/region/r.0.0.mca", 9.4 * MB, 0.01),
    b("/world/region/r.-1.0.mca", 7.1 * MB, 0.01),
    t("/logs/latest.log", "[08:12:01] [Server thread/INFO]: Done (4.211s)! (synthetic)\n", 0.2),
  ],
  prom_data: [b("/chunks_head/000123", 128 * MB, 0.01), b("/wal/00000042", 96 * MB, 0.01), t("/lock", "", 1), d("/01J9X3V2K8QYQ7")],
  grafana_data: [b("/grafana.db", 18 * MB, 0.1), d("/plugins"), d("/csv")],
  adguard_work: [t("/data/querylog.json", '{"QH":"example.com","QT":"A","Result":{}}\n', 0.01), b("/data/stats.db", 6 * MB, 0.05), b("/data/sessions.db", 32 * KB, 0.5)],
  adguard_conf: [t("/AdGuardHome.yaml", "# Synthetic AdGuard Home config\nhttp:\n  address: 0.0.0.0:80\ndns:\n  upstream_dns:\n    - 192.0.2.53\n", 6)],
  forgejo_data: [
    t("/gitea/conf/app.ini", "APP_NAME = Forgejo (Beispiel)\n[server]\nDOMAIN = git.example.com\n[database]\nDB_TYPE = sqlite3\n", 15),
    b("/gitea/forgejo.db", 4.2 * MB, 3),
    t("/git/repositories/alex/dotfiles.git/HEAD", "ref: refs/heads/main\n", 20),
    b("/git/repositories/alex/dotfiles.git/objects/pack/pack-1.pack", 1.6 * MB, 20),
  ],
  netdata_config: [t("/netdata.conf", "# Synthetic netdata.conf\n[global]\n  update every = 2\n", 3)],
  kuma_data: [b("/kuma.db", 42 * MB, 0.02), d("/upload")],
};

const BIND_BY_PATH: Record<string, FileSeed[]> = {
  "/mnt/user/media": [
    b("/Filme/Beispielfilm (2024)/Beispielfilm (2024).mkv", 4.2 * GB, 90),
    b("/Filme/Kurzfilm (2023)/Kurzfilm (2023).mkv", 1.1 * GB, 200),
    b("/Serien/Beispielserie/Staffel 01/S01E01.mkv", 1.4 * GB, 120),
    b("/Serien/Beispielserie/Staffel 01/S01E02.mkv", 1.3 * GB, 120),
    b("/Musik/Beispielband/Album/01 - Intro.flac", 28 * MB, 300),
  ],
  "/mnt/user/scans": [b("/scan_2026-10-09_0001.pdf", 1.2 * MB, 1), b("/scan_2026-10-09_0002.pdf", 860 * KB, 1), d("/fehlgeschlagen")],
  "/mnt/user/appdata/duplicati": [b("/Duplicati-server.sqlite", 3.4 * MB, 0.1), b("/control_dir_v2/lock_v2", 0, 0.1)],
  "/mnt/user/backups": [b("/duplicati/duplicati-20261009T030000Z.dlist.zip", 12 * MB, 1), b("/duplicati/duplicati-b1f.dblock.zip", 48 * MB, 1)],
  "/mnt/user": [d("/appdata"), d("/backups"), d("/media"), d("/scans"), t("/LIESMICH.txt", "Synthetischer Unraid-Share für den Demo-Dateibrowser.\n", 50)],
};

const PROMETHEUS_YML = "# Synthetic prometheus.yml\nglobal:\n  scrape_interval: 15s\nscrape_configs:\n  - job_name: node\n    static_configs:\n      - targets: ['node-exporter:9100']\n";

/** Seed tree for a storage entry; unknown entries get a small generic tree. */
export function seedFiles(entry: { kind: "volume" | "bind"; name: string; composeName?: string; fileBind?: boolean }): FileSeed[] {
  if (entry.kind === "bind") {
    if (entry.fileBind) {
      const file = entry.name.split("/").pop() ?? "datei.txt";
      return [t(`/${file}`, file === "prometheus.yml" ? PROMETHEUS_YML : `# ${file} (synthetisch)\n`, 4)];
    }
    return BIND_BY_PATH[entry.name] ?? [t("/LIESMICH.txt", "Synthetischer Bind-Mount.\n", 10)];
  }
  const key = entry.composeName ?? entry.name;
  return BY_KEY[key] ?? [];
}
