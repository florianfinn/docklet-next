/* Synthetic Compose files. Unknown keys (x-*, labels, deploy) are intentional
   so the editor can demonstrate lossless round trips. */

export const COMPOSE: Record<string, string> = {
  media: `# Media-Stack (synthetisch)
name: media
x-docklet:
  owner: familie
services:
  jellyfin:
    image: jellyfin/jellyfin:10.10.3
    container_name: jellyfin
    restart: unless-stopped
    ports:
      - "8096:8096"
    volumes:
      - media_config:/config
      - /mnt/user/media:/media:ro
    environment:
      TZ: Europe/Berlin
      JELLYFIN_PublishedServerUrl: https://media.example.com
    depends_on:
      - redis
    healthcheck:
      test: ["CMD-SHELL", "curl -fs http://localhost:8096/health || exit 1"]
      interval: 30s
      retries: 3
    deploy:
      resources:
        limits:
          memory: 4g
  redis:
    image: redis:7.4-alpine
    restart: unless-stopped
    volumes:
      - redis_data:/data
volumes:
  media_config:
  redis_data:
`,
  paperless: `# Dokumente (synthetisch)
name: paperless
services:
  webserver:
    image: ghcr.io/paperless-ngx/paperless-ngx:2.18.4
    restart: unless-stopped
    ports:
      - "8010:8000"
    volumes:
      - paperless_data:/usr/src/paperless/data
      - /mnt/user/scans:/usr/src/paperless/consume
    environment:
      PAPERLESS_REDIS: redis://broker:6379
      PAPERLESS_DBHOST: db
      PAPERLESS_URL: https://docs.example.com
  db:
    image: postgres:17-alpine
    restart: unless-stopped
    volumes:
      - pg_data:/var/lib/postgresql/data
    environment:
      POSTGRES_DB: paperless
      POSTGRES_PASSWORD: \${DB_PASSWORD}
  broker:
    image: redis:7.4-alpine
    restart: unless-stopped
    # Synthetic example of one explicit edge: broker waits for webserver, not for db.
    depends_on: [webserver]
volumes:
  paperless_data:
  pg_data:
`,
  home: `# Smart Home (synthetisch)
name: home
services:
  homeassistant:
    image: ghcr.io/home-assistant/home-assistant:2026.9.3
    restart: unless-stopped
    network_mode: host
    volumes:
      - ha_config:/config
    environment:
      TZ: Europe/Berlin
    privileged: true
  mosquitto:
    image: eclipse-mosquitto:2.0.22
    restart: unless-stopped
    ports:
      - "1883:1883"
    volumes:
      - mqtt_data:/mosquitto/data
volumes:
  ha_config:
  mqtt_data:
`,
  cloud: `# Cloud (synthetisch)
name: cloud
services:
  nextcloud:
    image: nextcloud:31.0.9-apache
    restart: unless-stopped
    ports:
      - "8443:80"
    volumes:
      - cloud_data:/var/www/html
    environment:
      POSTGRES_HOST: postgres
      POSTGRES_PASSWORD: \${DB_PASSWORD}
      NEXTCLOUD_TRUSTED_DOMAINS: cloud.example.com
    networks: [web, internal]
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    volumes:
      - cloud_pg:/var/lib/postgresql/data
    environment:
      - POSTGRES_PASSWORD=\${DB_PASSWORD}
    networks: [internal]
  cache:
    image: redis:7.4-alpine
    restart: unless-stopped
    networks: [internal]
networks:
  web: {}
  internal:
    internal: true
volumes:
  cloud_data:
  cloud_pg:
`,
  craft: `# Minecraft (synthetisch)
name: craft
x-docklet-game:
  console: rcon
services:
  minecraft:
    image: itzg/minecraft-server:2025.9.0
    restart: unless-stopped
    ports:
      - "25565:25565"
    volumes:
      - craft_world:/data
    environment:
      EULA: "TRUE"
      MEMORY: 6G
    healthcheck:
      test: ["CMD", "mc-health"]
      interval: 60s
      start_period: 2m
    stop_grace_period: 1m
volumes:
  craft_world:
`,
  monitoring: `# Monitoring (synthetisch)
name: monitoring
services:
  prometheus:
    image: prom/prometheus:latest
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prom_data:/prometheus
  grafana:
    image: grafana/grafana:12.1.1
    restart: unless-stopped
    ports:
      - "3000:3000"
    volumes:
      - grafana_data:/var/lib/grafana
    environment:
      GF_SERVER_ROOT_URL: https://grafana.example.com
  node-exporter:
    image: prom/node-exporter:v1.9.1
    restart: unless-stopped
    network_mode: host
    pid: host
volumes:
  prom_data:
  grafana_data:
`,
  dns: `# DNS (synthetisch)
name: dns
services:
  adguard:
    image: adguard/adguardhome:v0.107.65
    restart: unless-stopped
    ports:
      - "53:53/udp"
      - "3080:80"
    volumes:
      - adguard_work:/opt/adguardhome/work
      - adguard_conf:/opt/adguardhome/conf
  unbound:
    image: mvance/unbound-rpi:1.22.0
    restart: unless-stopped
volumes:
  adguard_work:
  adguard_conf:
`,
  git: `# Git (synthetisch, gestoppt)
name: git
services:
  forgejo:
    image: codeberg.org/forgejo/forgejo:12.0.2
    restart: unless-stopped
    ports:
      - "3001:3000"
      - "2222:22"
    volumes:
      - forgejo_data:/data
volumes:
  forgejo_data:
`,
  wiki: `# Wiki (synthetisch)
name: wiki
services:
  wikijs:
    image: ghcr.io/requarks/wiki:2.5.308
    restart: unless-stopped
    ports:
      - "3002:3000"
    environment:
      DB_TYPE: postgres
      DB_HOST: db
  db:
    image: postgres:16-alpine
    restart: unless-stopped
    volumes:
      - wiki_pg:/var/lib/postgresql/data
volumes:
  wiki_pg:
`,
};

export const ENV_FILES: Record<string, string> = {
  media: "TZ=Europe/Berlin\n",
  paperless: "DB_PASSWORD=bitte-lokal-setzen\n",
  home: "",
  cloud: "DB_PASSWORD=bitte-lokal-setzen\n",
  craft: "",
  monitoring: "",
  dns: "",
  git: "",
  wiki: "",
};
