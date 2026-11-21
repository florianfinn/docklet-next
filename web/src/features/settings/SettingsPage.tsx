import { ModulesSection } from "./sections/ModulesSection";
import { IDS } from "./settings-sections";
import { Archive, BellRing, HardDrive, HeartPulse, History, Info, KeyRound, LayoutGrid, Palette, RefreshCw, Settings2, ShieldAlert } from "lucide-react";
import { NavLink, Navigate, useNavigate, useParams } from "react-router";
import { useTranslations } from "use-intl";

import { useConnectedSettings, useSettings, useHubVersionHint } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { PageHeader, QueryError } from "@/shared/components/page";
import { FavoritesSection } from "@/shared/preferences/FavoritesSection";
import { HubUpdateHint } from "@/shared/components/hub-update/HubUpdateHint";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Skeleton } from "@/shared/ui/skeleton";

import { AuthSection } from "./sections/AuthSection";
import { ConnectedAuthSection } from "./sections/ConnectedAuthSection";
import { BackupTargetsSection } from "./sections/BackupTargetsSection";
import { GeneralSection } from "./sections/GeneralSection";
import { HelpSection } from "./sections/HelpSection";
import { AutoHealSection } from "./sections/AutoHealSection";
import { NotificationsSection } from "./sections/NotificationsSection";
import { SecurityLiveSection } from "./sections/SecurityLiveSection";
import { ServiceWidgetsSection } from "./sections/ServiceWidgetsSection";
import { SecuritySection } from "./sections/SecuritySection";
import { AppearanceSection } from "./sections/AppearanceSection";
import { ActivitySection, HostDefaultsSection, ConnectedHostDefaultsSection } from "./sections/SimpleSections";
import { ConnectedUpdatesSection as LiveUpdatesSection } from "./sections/ConnectedUpdatesSection";
import { UpdatesSection } from "./sections/UpdatesSection";

const GROUPS = [
  { key: "hub", items: [{ id: "modules", icon: LayoutGrid }, { id: "general", icon: Settings2 }, { id: "appearance", icon: Palette }, { id: "help", icon: Info }, { id: "auth", icon: KeyRound }] },
  { key: "operations", items: [{ id: "hosts", icon: HardDrive }, { id: "updates", icon: RefreshCw }, { id: "backups", icon: Archive }, { id: "notifications", icon: BellRing }, { id: "autoHeal", icon: HeartPulse }, { id: "serviceWidgets", icon: LayoutGrid }] },
  { key: "system", items: [{ id: "security", icon: ShieldAlert }, { id: "activity", icon: History }] },
] as const;

type SectionId = (typeof GROUPS)[number]["items"][number]["id"];

/** Hub settings with grouped sub-navigation; each section saves on its own. */
export default function SettingsPage() {
  const { section } = useParams();
  const navigate = useNavigate();
  const t = useTranslations("settings");
  const tHub = useTranslations("common.hubUpdate");
  const adapter = useAdapter();
  const hubVersion = useHubVersionHint();
  const validSection = !!section && (IDS as readonly string[]).includes(section);
  const id = validSection ? section as SectionId : "general";
  const connected = adapter.kind === "http" && (id === "general" || id === "auth" || id === "activity");
  const liveHosts = adapter.kind === "http" && id === "hosts";
  const liveScans = adapter.kind === "http" && id === "security";
  const liveBackups = id === "backups";
  // Notification targets and Auto Heal have no hub-settings draft; both adapters serve them directly.
  const own = id === "modules" || id === "notifications" || id === "autoHeal" || id === "serviceWidgets";
  const liveUpdates = adapter.kind === "http" && id === "updates";
  const settings = useSettings(!liveHosts && !liveScans && !liveUpdates && !liveBackups && !own && !connected && id !== "appearance" && id !== "help");
  const hub = useConnectedSettings(connected && id !== "auth");
  const s = settings.data;
  if (!validSection) return <Navigate to="/settings/general" replace />;

  return (
    <>
      <PageHeader title={t("title")} description={t("description")} />
      <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <nav aria-label={t("navLabel")} className="min-w-0 lg:sticky lg:top-20">
          {/* Narrow screens: one grouped section picker; the sidebar list starts at lg. */}
          <div className="lg:hidden">
            <Select value={id} onValueChange={(next) => void navigate(`/settings/${next}${next === "updates" && hubVersion ? "#hub-update" : ""}`)}>
              <SelectTrigger aria-label={t("navLabel")} className="w-full" data-testid="settings-section-picker"><SelectValue /></SelectTrigger>
              <SelectContent>
                {GROUPS.map((g) => (
                  <SelectGroup key={g.key}>
                    <SelectLabel>{t(`groups.${g.key}`)}</SelectLabel>
                    {g.items.map(({ id: item, icon: Icon }) => (
                      <SelectItem key={item} value={item}>
                        <Icon className="size-4" aria-hidden />
                        {t(`sections.${item}`)}
                        {item === "updates" && hubVersion && <><span aria-hidden className="size-2 rounded-full bg-primary" /><span className="sr-only">{tHub("available", { version: hubVersion })}</span></>}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="hidden lg:flex lg:flex-col lg:gap-4" data-testid="settings-section-list">
            {GROUPS.map((g) => (
              <div key={g.key} className="flex flex-col items-stretch gap-1">
                <div className="px-2.5 pb-1 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{t(`groups.${g.key}`)}</div>
                {g.items.map(({ id: item, icon: Icon }) => (
                  <NavLink
                    key={item}
                    to={`/settings/${item}${item === "updates" && hubVersion ? "#hub-update" : ""}`}
                    end
                    className="nav-row flex shrink-0 items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm whitespace-nowrap outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                  >
                    <Icon className="size-4" aria-hidden />
                    {t(`sections.${item}`)}{item === "updates" && <HubUpdateHint version={hubVersion} />}
                  </NavLink>
                ))}
              </div>
            ))}
          </div>
        </nav>
        <div key={id} className="min-w-0">
          {id === "modules" ? <ModulesSection /> : id === "appearance" ? (
            <div className="flex flex-col gap-5">
              <AppearanceSection />
              <div className="rounded-xl border bg-card p-6">
                <FavoritesSection />
              </div>
            </div>
          ) : id === "help" ? (
            <HelpSection />
          ) : id === "serviceWidgets" ? <ServiceWidgetsSection /> : id === "notifications" ? <NotificationsSection /> : id === "autoHeal" ? <AutoHealSection /> : liveHosts ? <ConnectedHostDefaultsSection /> : liveBackups ? <BackupTargetsSection /> : liveScans ? <SecurityLiveSection /> : liveUpdates ? <LiveUpdatesSection /> : connected ? (
            id === "auth" ? <ConnectedAuthSection /> : hub.isError ? <QueryError error={hub.error} onRetry={() => void hub.refetch()} /> : !hub.data ? (
              <div className="flex flex-col gap-3" aria-busy="true"><Skeleton className="h-16 rounded-2xl" /><Skeleton className="h-80 rounded-2xl" /></div>
            ) : id === "general" ? <GeneralSection settings={hub.data} /> : <ActivitySection settings={hub.data} />
          ) : settings.isError ? (
            <QueryError error={settings.error} onRetry={() => void settings.refetch()} />
          ) : !s ? (
            <div className="flex flex-col gap-3" aria-busy="true">
              <Skeleton className="h-16 rounded-2xl" />
              <Skeleton className="h-80 rounded-2xl" />
            </div>
          ) : id === "general" ? (
            <GeneralSection settings={s} />
          ) : id === "auth" ? (
            <AuthSection settings={s} />
          ) : id === "hosts" ? (
            <HostDefaultsSection settings={s} />
          ) : id === "updates" ? (
            <UpdatesSection settings={s} />
          ) : id === "security" ? (
            <SecuritySection settings={s} />
          ) : (
            <ActivitySection settings={s} />
          )}
        </div>
      </div>
    </>
  );
}
