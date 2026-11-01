// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/app/shell/free/FreeUserMenu.tsx
// Revision: 15d018102fa64d9ac7fdd2b572848101f2495a8112d235dc3375056757d8476b
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: M2 production shell: disconnected error handling, development-only demo boundary, neutral product text or responsive layout. Historical import recorded in web/PROVENANCE.md. #69 connected identity production routes and catalogues. Simple-user management, assigned-stack access and mode-aware self-service.

import { ChevronsUpDown, Languages, LogOut, Moon, Settings2, Sun, UserRound } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { LOCALES, setAppLocale, useAppLocale } from "@/app/i18n/locale-store";
import { useAdapter } from "@/data/use-adapter";
import { useAuthMode, useCurrentUser } from "@/data/queries";
import { setTokens } from "@/shared/theme/design-tokens";
import { useTokens } from "@/shared/theme/use-design-tokens";
import { LabMenuItem } from "@/runtime";
import { Avatar, AvatarFallback } from "@/shared/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/shared/ui/dropdown-menu";
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem, useSidebar } from "@/shared/ui/sidebar";

import { LogoutDialog } from "../LogoutDialog";

/** Sidebar account menu for the connected user or the demo administrator. */
export function FreeUserMenu({ onNavigate }: { onNavigate: () => void }) {
  const t = useTranslations("shell.user");
  const tl = useTranslations("locale.switcher");
  const locale = useAppLocale();
  const navigate = useNavigate();
  const tokens = useTokens();
  const { isMobile } = useSidebar();
  const [logout, setLogout] = useState(false);
  const disconnected = useAdapter().kind === "disconnected";
  const adapter = useAdapter();
  const mode = useAuthMode();
  const currentUser = useCurrentUser(adapter.kind === "http");
  const connected = adapter.kind === "http";
  const name = connected ? currentUser.data?.username ?? t("unknownName") : t(disconnected ? "unknownName" : "name");
  const detail = connected ? t(currentUser.data?.isAdmin === false ? "userRole" : "role") : t(disconnected ? "unknownStatus" : "email");
  const initials = name
    .split(" ")
    .map((part) => part[0])
    .join("");
  const go = (to: string) => {
    onNavigate();
    navigate(to);
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton size="lg" aria-label={t("menu")} className="data-[state=open]:bg-sidebar-accent">
              <Avatar className="size-8 rounded-lg">
                <AvatarFallback className="rounded-lg bg-primary/15 text-xs font-semibold text-primary-text">{initials}</AvatarFallback>
              </Avatar>
              <span className="grid flex-1 text-left leading-tight">
                <span className="truncate text-sm font-medium">{name}</span>
                <span className="truncate text-xs text-muted-foreground">{detail}</span>
              </span>
              <ChevronsUpDown className="ml-auto size-4 text-muted-foreground" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent side={isMobile ? "top" : "right"} align="end" sideOffset={10} className="w-64">
            {!disconnected && <DropdownMenuLabel className="font-normal">
              <span className="block text-xs text-muted-foreground">{t("signedInAs")}</span>
              <span className="block truncate text-sm font-medium">{connected ? name : t("email")}</span>
              <span className="block truncate text-xs text-muted-foreground">{t(currentUser.data?.isAdmin === false ? "userRole" : "role")}</span>
            </DropdownMenuLabel>}
            {!disconnected && <DropdownMenuSeparator />}
            <DropdownMenuItem onSelect={() => go("/profile")}>
              <UserRound />
              {t("profile")}
            </DropdownMenuItem>
            {currentUser.data?.isAdmin !== false && <DropdownMenuItem onSelect={() => go("/settings")}>
              <Settings2 />
              {t("settings")}
            </DropdownMenuItem>}
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="py-1 text-xs font-normal text-muted-foreground">{t("theme")}</DropdownMenuLabel>
            <DropdownMenuRadioGroup value={tokens.theme} onValueChange={(v) => setTokens({ theme: v === "light" ? "light" : "dark" })}>
              <DropdownMenuRadioItem value="dark">
                <Moon />
                {t("dark")}
              </DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="light">
                <Sun />
                {t("light")}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
            <DropdownMenuLabel className="py-1 text-xs font-normal text-muted-foreground">{tl("label")}</DropdownMenuLabel>
            {connected ? <DropdownMenuItem onSelect={() => go("/profile")}><Languages />{tl("label")}</DropdownMenuItem> : <DropdownMenuRadioGroup value={locale} onValueChange={(v) => setAppLocale(v === "en-US" ? "en-US" : "de")}>
              {LOCALES.map((l) => (
                <DropdownMenuRadioItem key={l} value={l} lang={l}>
                  <Languages />
                  {tl(`names.${l === "de" ? "de" : "enUS"}`)}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>}
            {!disconnected && <DropdownMenuSeparator />}
            <LabMenuItem />
            {!disconnected && (mode.data === "local" || mode.data === "proxy") && <DropdownMenuItem variant="destructive" onSelect={() => setLogout(true)}>
              <LogOut />
              {t("logout")}
            </DropdownMenuItem>}
          </DropdownMenuContent>
        </DropdownMenu>
        {!disconnected && <LogoutDialog open={logout} onOpenChange={setLogout} />}
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
