<!--
Source: docklet-next/issues/274
Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
Retrieved: 2026-11-07
License: Apache-2.0
Deviations: Original reverse-proxy requirements for the hub.
-->

# Running the hub behind a reverse proxy

The hub listens on loopback (`LISTEN=127.0.0.1`, `PORT=3552`) and expects an operator reverse proxy
in front of it for TLS and the public name. This page lists what that proxy must forward. Examples use
`docklet.example.com`; replace it with your own name.

## Requirements

- **Dedicated name at the domain root.** Serve the hub at `https://docklet.example.com/`. Sub-path
  hosting (`https://example.com/docklet/`) is unsupported for assets, routing, API, WebSockets, sign-in
  and links. Set `APP_URL=https://docklet.example.com` exactly. Browser requests that change state must
  carry this exact `Origin`; the hub refuses others with 403.
- **WebSocket upgrades** on `/api/environments/*/ws/...` (container logs, stats and terminal, project
  logs, system stats). Forward `Upgrade` and `Connection` headers.
- **Unbuffered server-sent events** on `GET /api/stream`. Live updates (activity progress, finished
  actions, host state) arrive over this stream. The hub sends `Content-Type: text/event-stream`,
  `Cache-Control: no-store, no-cache, must-revalidate` and `X-Accel-Buffering: no`, and a heartbeat every
  15 seconds. A proxy that buffers the response delivers events in clusters: pages then show running
  actions or old counts for seconds to minutes after the hub finished them. Keep the read timeout on this
  route at 60 seconds or more so the heartbeat holds the connection open.
- **HTTP/2 between browser and proxy is recommended.** Over HTTP/1.1 a browser opens at most six
  connections per name. When one agent stops answering, its slow reads can hold those connections, and
  pages for healthy hosts wait until the hub's 20 second inventory bound or the browser's 30 second
  per-host budget ends those reads. With HTTP/2 the reads share one connection and other pages keep
  loading.
- **Forwarded headers.** Send `X-Forwarded-For` and `X-Forwarded-Proto`. List the proxy's address in
  `TRUSTED_PROXIES` so the hub records the client address; this setting does not grant sign-in trust
  (see `AUTH_MODE=proxy` in [deploy/hub/README.md](../../deploy/hub/README.md) for proxy sign-in).
- **Long downloads.** A volume or bind snapshot download prepares the whole archive before the first
  byte. Large snapshots can take minutes. A proxy with a short first-byte or read timeout ends such a
  download with 504. Raise the read timeout for the hub (for example to 10 minutes), or restore the
  snapshot in place instead of downloading it.
- **Encoded path characters.** The hub refuses addresses under `/api/environments/` that contain encoded
  `/ ? # % \`, encoded letters, digits or `- . _ ~`, or encoded whitespace and control characters, with
  400 `request.path-encoded`. Do not let the proxy re-encode or decode request paths.

## Nginx or OpenResty

```nginx
server {
    listen 443 ssl;
    http2 on;
    server_name docklet.example.com;

    location / {
        proxy_pass http://127.0.0.1:3552;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 600s;
    }

    location = /api/stream {
        proxy_pass http://127.0.0.1:3552;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 120s;
    }
}

# In the http block:
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}
```

## Caddy

Caddy forwards WebSocket upgrades and flushes `text/event-stream` responses without further
configuration.

```caddy
docklet.example.com {
    reverse_proxy 127.0.0.1:3552
}
```

## Traefik

Do not attach a buffering middleware to the hub's router; Traefik streams responses by default.
Traefik forwards WebSocket upgrades without extra settings.

```yaml
http:
  routers:
    docklet:
      rule: Host(`docklet.example.com`)
      entryPoints: [websecure]
      service: docklet
      tls: {}
  services:
    docklet:
      loadBalancer:
        servers:
          - url: http://127.0.0.1:3552
```

For long snapshot downloads, keep the entry point's `respondingTimeouts.writeTimeout` and the servers
transport's `responseHeaderTimeout` unset (their default) or above the snapshot preparation time.

## Checking the setup

- Open the dashboard, start an action and watch it finish: the activity should leave the running state
  within about two seconds of finishing, without a page reload.
- Open a container's logs or terminal: the stream should connect and stay open.
- In the browser developer tools, the protocol column for hub requests should show `h2`.
