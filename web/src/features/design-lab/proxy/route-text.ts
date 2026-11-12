// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { proxyStore, routeFromYaml, routeYaml, type ProxyRoute } from "./model";
import { routeCaddyfile, routeFromCaddyfile } from "./caddyfile";
export const instanceKind = (id: string) => proxyStore.get().instances.find((p) => p.id === id)?.kind ?? "traefik";
export const routeText = (route: ProxyRoute) => instanceKind(route.proxy) === "caddy" ? routeCaddyfile(route) : routeYaml(route);
export const routeFromText = (text: string, route: ProxyRoute) => instanceKind(route.proxy) === "caddy" ? routeFromCaddyfile(text, route) : routeFromYaml(text, route);
