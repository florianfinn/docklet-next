import { Activity } from "lucide-react";
import { NavLink, useLocation } from "react-router";
import { useTranslations } from "use-intl";

import { openActivity } from "@/features/activity/activity-store";
import { useMotionEnabled } from "@/features/design-lab/use-design-lab";
import { BrandLogo } from "@/shared/brand/BrandLogo";
import { cn } from "@/shared/lib/utils";
import { useVisualStyle } from "@/shared/preferences/visual-style-store";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from "@/shared/ui/sidebar";

import { HostFilter } from "../HostFilter";
import { NAV, type NavItem } from "../nav-items";
import { isNavActive, useNavCounts, useRunningActionCount, type NavCounts } from "../use-nav-counts";
import { FreeUserMenu } from "./FreeUserMenu";

/** Shared navigation row look (controls.css), also used by the settings sub-navigation. */
const ROW = "nav-row group/free-nav";

function NavEntry({ item, count, active, onNavigate, groupBadge }: { item: NavItem; count: NavCounts[keyof NavCounts]; active: boolean; onNavigate: () => void; groupBadge?: boolean }) {
  const t = useTranslations("shell");
  const label = t(`nav.${item.key}`);
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={label} className={ROW}>
        <NavLink to={item.to} onClick={onNavigate} data-preview={item.preview || undefined}>
          <item.icon />
          <span>
            {label}
            {item.preview && <span className="sr-only"> ({t("preview")})</span>}
          </span>
        </NavLink>
      </SidebarMenuButton>
      {count && !item.preview ? (
        <SidebarMenuBadge className={cn("font-normal tabular-nums", count.tone === "err" ? "text-destructive" : "text-muted-foreground")}>{count.value}</SidebarMenuBadge>
      ) : item.preview && !groupBadge ? (
        <SidebarMenuBadge aria-hidden className="h-4 rounded-full border px-1.5 text-[10px] font-normal text-muted-foreground">
          {t("preview")}
        </SidebarMenuBadge>
      ) : null}
    </SidebarMenuItem>
  );
}

/**
 * Shell sidebar: floating, rounded shadcn/ui Sidebar with the Docklet Next logo,
 * grouped navigation, clearly marked active rows and counters, and the profile at the
 * bottom. Own composition on the free primitives.
 */
export function FreeSidebar() {
  const t = useTranslations("shell");
  const ta = useTranslations("activity");
  const counts = useNavCounts();
  const running = useRunningActionCount();
  const { pathname } = useLocation();
  const tv = useTranslations("visual.brand");
  const { isMobile, setOpenMobile, state } = useSidebar();
  const collapsed = state === "collapsed" && !isMobile;
  const logoVariant = useVisualStyle("logoVariant");
  const logoMotion = useVisualStyle("logoMotion");
  const logoEntrance = useVisualStyle("logoEntrance");
  const motion = useMotionEnabled();
  const closeMobile = () => {
    if (isMobile) setOpenMobile(false);
  };

  return (
    <Sidebar variant="floating" collapsible="icon">
      <SidebarHeader data-slot="free-sidebar-header">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild tooltip={t("brand")} className="group-data-[collapsible=icon]:justify-center">
              <NavLink to="/dashboard" onClick={closeMobile} aria-label={tv("home")} data-slot="brand-link">
                <BrandLogo
                  variant={collapsed ? "mark" : logoVariant}
                  motionStyle={logoMotion}
                  size={collapsed ? 22 : 34}
                  entranceSlot={logoEntrance ? "sidebar" : undefined}
                  motion={motion}
                  decorative
                />
              </NavLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <HostFilter />
        <SidebarGroup className="py-0">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={ta("open")}
                className={ROW}
                aria-label={running ? ta("openWithCount", { count: running }) : ta("open")}
                onClick={() => {
                  closeMobile();
                  openActivity();
                }}
              >
                <Activity className={running ? "!text-primary-text" : undefined} />
                <span>{ta("open")}</span>
              </SidebarMenuButton>
              {running > 0 && (
                <SidebarMenuBadge aria-hidden className="font-normal text-primary-text tabular-nums">
                  {running}
                </SidebarMenuBadge>
              )}
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
        {NAV.map((group) => (
          <SidebarGroup key={group.key}>
            <SidebarGroupLabel className="text-[11px] font-medium tracking-wide">
              {t(`groups.${group.key}`)}
              {group.key === "later" && <span className="ml-1.5 rounded-full border px-1.5 text-[10px] font-normal">{t("preview")}</span>}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {group.items.map((item) => (
                  <NavEntry key={item.key} item={item} groupBadge={group.key === "later"} count={counts[item.key]} active={isNavActive(pathname, item.to)} onNavigate={closeMobile} />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SidebarFooter>
        <FreeUserMenu onNavigate={closeMobile} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
