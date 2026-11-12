import {
  Archive,
  BellRing,
  Boxes,
  Database,
  Gamepad2,
  LayoutDashboard,
  Layers,
  Network,
  Package,
  Server,
  Shield,
  Waypoints,
  type LucideIcon,
} from "lucide-react";
import { matchRoutes, type RouteObject } from "react-router";

export type NavKey =
  | "dashboard"
  | "stacks"
  | "containers"
  | "security"
  | "networks"
  | "registries"
  | "images"
  | "volumes"
  | "hosts"
  | "backups"
  | "hubBackups"
  | "notifications"
  | "proxy"
  | "games";

export interface NavItem {
  key: NavKey;
  to: string;
  icon: LucideIcon;
  /** Planned area with no active feature controls. */
  comingSoon?: boolean;
}

export interface NavGroup {
  key: "operate" | "resources" | "system" | "later";
  items: NavItem[];
}

export const NAV: NavGroup[] = [
  {
    key: "operate",
    items: [
      { key: "dashboard", to: "/dashboard", icon: LayoutDashboard },
      { key: "stacks", to: "/stacks", icon: Layers },
      { key: "containers", to: "/containers", icon: Boxes },
      { key: "security", to: "/security", icon: Shield },
    ],
  },
  {
    key: "resources",
    items: [
      { key: "networks", to: "/networks", icon: Network },
      { key: "volumes", to: "/volumes", icon: Database },
      { key: "images", to: "/images", icon: Package },
      { key: "registries", to: "/registries", icon: Package },
    ],
  },
  {
    key: "system",
    items: [
      { key: "hosts", to: "/hosts", icon: Server },
      { key: "backups", to: "/backups", icon: Archive },
      { key: "hubBackups", to: "/hub-backups", icon: Database },
      { key: "notifications", to: "/settings/notifications", icon: BellRing },
    ],
  },
  {
    key: "later",
    items: [
      { key: "proxy", to: "/proxy", icon: Waypoints, comingSoon: true },
      { key: "games", to: "/games", icon: Gamepad2, comingSoon: true },
    ],
  },
];

export function navigationForRoutes(routes: RouteObject[]): NavGroup[] {
  return NAV.map((group) => ({
    ...group,
    items: group.items.filter((item) => {
      const matches = matchRoutes(routes, item.to);
      return matches !== null && matches.every(({ route }) => route.path !== "*");
    }),
  })).filter((group) => group.items.length > 0);
}

export function visibleNavItems(items: NavItem[], canReadHubBackups: boolean): NavItem[] {
  return items.filter((item) => item.key !== "hubBackups" || canReadHubBackups);
}
