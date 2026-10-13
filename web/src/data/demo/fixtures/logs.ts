import type { LogLevel } from "@/domain/model";

type Template = [LogLevel, string];

/** Message templates per service name; the log simulator samples from these. */
export const LOG_TEMPLATES: Record<string, Template[]> = {
  jellyfin: [
    ["info", "[INF] Kestrel listening on 0.0.0.0:8096"],
    ["info", "[INF] Playback started: Film (Direct Play, 1080p)"],
    ["info", "[INF] Scheduled task \"Scan Media Library\" completed after 0 minutes"],
    ["warn", "[WRN] Slow response from metadata provider (2.4 s)"],
    ["debug", "[DBG] Image cache hit ratio 0.97"],
  ],
  redis: [
    ["info", "DB saved on disk"],
    ["info", "Background saving started by pid 42"],
    ["info", "Ready to accept connections tcp"],
  ],
  broker: [
    ["info", "Background saving terminated with success"],
    ["info", "1 changes in 3600 seconds. Saving..."],
  ],
  cache: [["info", "DB saved on disk"]],
  webserver: [
    ["info", "[consumer] Consuming scan_0142.pdf"],
    ["info", "[tasks] Document scan_0142.pdf: OCR finished (3 pages)"],
    ["info", "[celery] Task documents.tasks.train_classifier succeeded"],
    ["warn", "[consumer] File already exists, skipping duplicate"],
  ],
  db: [
    ["info", "checkpoint starting: time"],
    ["info", "checkpoint complete: wrote 214 buffers (1.3%)"],
    ["info", "database system is ready to accept connections"],
  ],
  postgres: [
    ["info", "checkpoint complete: wrote 312 buffers (1.9%)"],
    ["info", "autovacuum: VACUUM public.oc_filecache"],
  ],
  homeassistant: [
    ["info", "Setup of domain mqtt took 0.4 seconds"],
    ["info", "Automation 'Licht Flur' triggered"],
    ["warn", "Update of sensor.balkon is taking over 10 seconds"],
  ],
  mosquitto: [
    ["info", "New client connected from 192.0.2.66 as homeassistant"],
    ["info", "Saving in-memory database to /mosquitto/data/mosquitto.db"],
  ],
  nextcloud: [
    ["info", "GET /status.php 200"],
    ["info", "cron.php: 14 Jobs ausgeführt"],
    ["warn", "Lange Antwortzeit für PROPFIND (1.8 s)"],
  ],
  minecraft: [
    ["warn", "[Server thread/WARN]: Can't keep up! Is the server overloaded? Running 2400ms behind"],
    ["error", "healthcheck: mc-health exit 1 (timeout)"],
    ["info", "[Server thread/INFO]: Spieler1 joined the game"],
    ["warn", "[Server thread/WARN]: Chunk save took 1820 ms"],
  ],
  prometheus: [
    ["info", "ts=… level=info component=tsdb msg=\"Head GC completed\" duration=12ms"],
    ["info", "ts=… level=info msg=\"Completed loading of configuration file\""],
  ],
  grafana: [
    ["info", "logger=context method=GET path=/api/health status=200"],
    ["info", "logger=cleanup msg=\"Completed cleanup jobs\""],
  ],
  "node-exporter": [["info", "level=info msg=\"Listening on\" address=[::]:9100"]],
  adguard: [
    ["info", "[info] dnsproxy: upstream unbound responded in 3 ms"],
    ["info", "[info] filtering: 412 Anfragen blockiert (letzte Stunde)"],
  ],
  unbound: [["info", "info: service started (unbound 1.22.0)"]],
};

export const FALLBACK_TEMPLATES: Template[] = [
  ["info", "Heartbeat ok"],
  ["info", "GET /health 200"],
];
