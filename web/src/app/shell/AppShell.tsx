import { StackUpdateDialogController } from "@/shared/components/updates/stack-update-dialog";
import { Activity } from "lucide-react";
import { Fragment, Suspense } from "react";
import { Link, Outlet, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { useActionRuns } from "@/data/live";
import { useContainers, useStacks, useVolume } from "@/data/queries";
import { ActionDialog } from "@/features/activity/ActionDialog";
import { ActionToasts } from "@/features/activity/ActionToasts";
import { ActivityCenter } from "@/features/activity/ActivityCenter";
import { openActivity } from "@/features/activity/activity-store";
import { AppToaster } from "@/features/activity/AppToaster";
import { DesignLabPanel, LiveToggle } from "@/runtime";
import { BackendNotice } from "@/data/BackendNotice";
import { deSettings } from "@/features/settings/messages/de";
import { ServiceRestartDialog } from "@/shared/components/service-restart";
import { sidebarWidthCss, useSidebarWidth } from "@/shared/preferences/visual-style-store";
import { Badge } from "@/shared/ui/badge";
import { Breadcrumb, BreadcrumbItem, BreadcrumbLink, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from "@/shared/ui/breadcrumb";
import { Button } from "@/shared/ui/button";
import { Separator } from "@/shared/ui/separator";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/shared/ui/sidebar";
import { Skeleton } from "@/shared/ui/skeleton";

import { PAGE_MAX_WIDTH, pageWidth } from "./page-width";
import { NAV, PREVIEW_ICONS, type NavGroup } from "./nav-items";
import { ShellEffects } from "./ShellEffects";
import { ShellSidebar } from "./ShellSidebar";

interface Crumb {
  label: string;
  to?: string;
}

function useCrumbs(): Crumb[] {
  const t = useTranslations("shell.nav");
  const ts = useTranslations("settings.sections");
  const tl = useTranslations("locale.ui");
  const { pathname } = useLocation();
  const [first, second] = pathname.split("/").filter(Boolean);
  const stacks = useStacks();
  const containers = useContainers();
  const volume = useVolume(first === "volumes" && second ? second : undefined);
  const known = ["dashboard", "stacks", "containers", "security", "networks", "images", "hosts", "volumes", "backups", "hub-backups", "settings", "profile"] as const;
  if (import.meta.env.DEV && first === "preview" && second && Object.hasOwn(PREVIEW_ICONS, second)) {
    const key = second as Parameters<typeof t>[0];
    return [{ label: t(key) }];
  }
  if (!first || !known.includes(first as (typeof known)[number])) return [];
  const root: Crumb = { label: first === "hub-backups" ? t("hubBackups") : t(first as Exclude<(typeof known)[number], "hub-backups">), to: `/${first}` };
  if (!second) return [{ label: root.label }];
  if (first === "stacks") {
    const name = second === "new" ? tl("newStack") : (stacks.data?.find((s) => s.id === second)?.name ?? second);
    return [root, { label: name }];
  }
  if (first === "hosts" && second === "new") return [root, { label: t("newHost") }];
  if (first === "containers") return [root, { label: containers.data?.find((c) => c.id === second)?.name ?? second }];
  if (first === "volumes") return [root, { label: volume.data?.composeName ?? volume.data?.name ?? "…" }];
  if (first === "settings" && second in deSettings.sections) return [root, { label: ts(second as keyof typeof deSettings.sections) }];
  return [{ label: root.label }];
}

function PageFallback() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-9 w-56" />
      <Skeleton className="h-4 w-96 max-w-full" />
      <Skeleton className="h-64 w-full rounded-2xl" />
    </div>
  );
}

export function AppShell({ navigation = NAV }: { navigation?: NavGroup[] }) {
  const t = useTranslations("shell");
  const ta = useTranslations("activity");
  const crumbs = useCrumbs();
  const location = useLocation();
  const running = useActionRuns().filter((r) => r.status === "queued" || r.status === "running").length;
  const width = pageWidth(location.pathname);
  const sidebarWidth = sidebarWidthCss(useSidebarWidth());

  return (
    <SidebarProvider style={{ "--sidebar-width": sidebarWidth } as React.CSSProperties}>
      <a href="#main-content" className="sr-only z-50 rounded-lg bg-primary px-3 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3">
        {t("skip")}
      </a>
      <ShellEffects />
      <ShellSidebar navigation={navigation} />
      <SidebarInset className="min-w-0 bg-transparent">
        <header className="glass sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b border-border/50 px-4">
          <SidebarTrigger className="-ml-1" aria-label={t("toggleSidebar")} />
          <Separator orientation="vertical" className="mr-1 data-[orientation=vertical]:h-4" />
          <Breadcrumb aria-label={t("breadcrumb")}>
            <BreadcrumbList>
              {crumbs.map((c, i) => (
                <Fragment key={c.label}>
                  {i > 0 && <BreadcrumbSeparator />}
                  <BreadcrumbItem>
                    {c.to && i < crumbs.length - 1 ? (
                      <BreadcrumbLink asChild>
                        <Link to={c.to}>{c.label}</Link>
                      </BreadcrumbLink>
                    ) : (
                      <BreadcrumbPage>{c.label}</BreadcrumbPage>
                    )}
                  </BreadcrumbItem>
                </Fragment>
              ))}
            </BreadcrumbList>
          </Breadcrumb>
          <div className="ml-auto flex items-center gap-1">
            <LiveToggle />
            <Button variant="ghost" size="sm" className="relative h-8 gap-1.5" onClick={() => openActivity()} aria-label={running ? ta("openWithCount", { count: running }) : ta("open")}>
              <Activity className={running ? "text-primary-text" : undefined} />
              <span className="hidden sm:inline">{ta("open")}</span>
              {running > 0 && (
                <Badge className="h-5 min-w-5 animate-pop-in rounded-full px-1.5 tabular-nums" aria-hidden>
                  {running}
                </Badge>
              )}
            </Button>
          </div>
        </header>
        <div id="main-content" tabIndex={-1} key={location.pathname} className="w-full min-w-0 flex-1 px-4 pt-6 pb-24 outline-none md:px-7 2xl:px-10">
          <div className="mx-auto w-full min-w-0" data-page-width={width} style={{ maxWidth: PAGE_MAX_WIDTH[width] }}>
            <BackendNotice />
            <Suspense fallback={<PageFallback />}>
              <Outlet />
          <StackUpdateDialogController />
            </Suspense>
          </div>
        </div>
      </SidebarInset>
      <ActivityCenter />
      <ActionToasts />
      <ActionDialog />
      <AppToaster />
      <ServiceRestartDialog />
      <DesignLabPanel />
    </SidebarProvider>
  );
}
