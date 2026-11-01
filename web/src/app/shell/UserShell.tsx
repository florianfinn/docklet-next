// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original reduced shell using the approved free sidebar primitives.

import { Layers } from "lucide-react";
import { NavLink, Outlet, useLocation } from "react-router";
import { Suspense } from "react";
import { useTranslations } from "use-intl";
import { ActionDialog } from "@/features/activity/ActionDialog";
import { ActionToasts } from "@/features/activity/ActionToasts";
import { AppToaster } from "@/features/activity/AppToaster";
import { StackUpdateDialogController } from "@/shared/components/updates/stack-update-dialog";
import { ServiceRestartDialog } from "@/shared/components/service-restart";
import { Sidebar, SidebarContent, SidebarFooter, SidebarHeader, SidebarInset, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarTrigger } from "@/shared/ui/sidebar";
import { FreeUserMenu } from "./free/FreeUserMenu";
import { ShellEffects } from "./ShellEffects";

export function UserShell() {
  const t = useTranslations("shell");
  const { pathname } = useLocation();
  const allowed = pathname === "/profile" || pathname === "/" || pathname === "/stacks" || /^\/stacks\/[^/]+$/.test(pathname) && pathname !== "/stacks/new";
  return <SidebarProvider><ShellEffects /><Sidebar variant="floating" collapsible="icon">
    <SidebarHeader><NavLink to="/stacks" className="p-3 font-semibold">{t("brand")}</NavLink></SidebarHeader>
    <SidebarContent><SidebarMenu><SidebarMenuItem><SidebarMenuButton asChild isActive={pathname.startsWith("/stacks")}><NavLink to="/stacks"><Layers />{t("nav.stacks")}</NavLink></SidebarMenuButton></SidebarMenuItem></SidebarMenu></SidebarContent>
    <SidebarFooter><FreeUserMenu onNavigate={() => {}} /></SidebarFooter>
  </Sidebar><SidebarInset className="min-w-0 bg-transparent"><header className="glass sticky top-0 z-30 flex h-14 items-center border-b px-4"><SidebarTrigger aria-label={t("toggleSidebar")} /></header>
    <main id="main-content" className="min-w-0 p-4 md:p-7"><Suspense fallback={null}>{allowed ? <Outlet /> : <div role="status"><h1 className="text-xl font-semibold">{t("unavailable.title")}</h1><p className="text-sm text-muted-foreground">{t("unavailable.body")}</p></div>}</Suspense></main>
  </SidebarInset>{allowed && <><ActionToasts /><ActionDialog /><ServiceRestartDialog /><StackUpdateDialogController /></>}<AppToaster /></SidebarProvider>;
}
