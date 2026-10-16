import { Archive, BellRing, HardDrive, History, Info, KeyRound, Palette, RefreshCw, Settings2, ShieldAlert } from "lucide-react";
import { NavLink, Navigate, useParams } from "react-router";
import { useTranslations } from "use-intl";

import { useConnectedSettings, useSettings } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { PageHeader, QueryError } from "@/shared/components/page";
import { FavoritesSection } from "@/shared/preferences/FavoritesSection";
import { Skeleton } from "@/shared/ui/skeleton";

import { AuthSection } from "./sections/AuthSection";
import { ConnectedAuthSection } from "./sections/ConnectedAuthSection";
import { BackupTargetsSection } from "./sections/BackupTargetsSection";
import { GeneralSection } from "./sections/GeneralSection";
import { HelpSection } from "./sections/HelpSection";
import { NotificationsSection } from "./sections/NotificationsSection";
import { SecuritySection } from "./sections/SecuritySection";
import { AppearanceSection } from "./sections/AppearanceSection";
import { ActivitySection, HostDefaultsSection } from "./sections/SimpleSections";
import { UpdatesSection } from "./sections/UpdatesSection";

const GROUPS = [
  { key: "hub", items: [{ id: "general", icon: Settings2 }, { id: "appearance", icon: Palette }, { id: "help", icon: Info }, { id: "auth", icon: KeyRound }] },
  { key: "operations", items: [{ id: "hosts", icon: HardDrive }, { id: "updates", icon: RefreshCw }, { id: "backups", icon: Archive }, { id: "notifications", icon: BellRing }] },
  { key: "system", items: [{ id: "security", icon: ShieldAlert }, { id: "activity", icon: History }] },
] as const;

type SectionId = (typeof GROUPS)[number]["items"][number]["id"];
const IDS = GROUPS.flatMap((g) => g.items.map((i) => i.id)) as SectionId[];

/** Hub settings with grouped sub-navigation; each section saves on its own. */
export default function SettingsPage() {
  const { section } = useParams();
  const t = useTranslations("settings");
  const adapter = useAdapter();
  const validSection = !!section && IDS.includes(section as SectionId);
  const id = validSection ? section as SectionId : "general";
  const connected = adapter.kind === "http" && (id === "general" || id === "auth" || id === "activity");
  const settings = useSettings(!connected && id !== "appearance" && id !== "help");
  const hub = useConnectedSettings(connected && id !== "auth");
  const s = settings.data;
  if (!validSection) return <Navigate to="/settings/general" replace />;

  return (
    <>
      <PageHeader title={t("title")} description={t("description")} />
      <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <nav aria-label={t("navLabel")} className="min-w-0 lg:sticky lg:top-20">
          {/* Narrow screens: the section links wrap into rows instead of a horizontal scroller. */}
          <div className="flex flex-wrap gap-x-4 gap-y-1 lg:flex-col lg:gap-4">
            {GROUPS.map((g) => (
              <div key={g.key} className="flex flex-wrap items-center gap-1 lg:flex-col lg:flex-nowrap lg:items-stretch">
                <div className="hidden px-2.5 pb-1 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase lg:block">{t(`groups.${g.key}`)}</div>
                {g.items.map(({ id: item, icon: Icon }) => (
                  <NavLink
                    key={item}
                    to={`/settings/${item}`}
                    end
                    className="nav-row flex shrink-0 items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm whitespace-nowrap outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                  >
                    <Icon className="size-4" aria-hidden />
                    {t(`sections.${item}`)}
                  </NavLink>
                ))}
              </div>
            ))}
          </div>
        </nav>
        <div key={id} className="min-w-0">
          {id === "appearance" ? (
            <div className="flex flex-col gap-5">
              <AppearanceSection />
              <div className="rounded-xl border bg-card p-6">
                <FavoritesSection />
              </div>
            </div>
          ) : id === "help" ? (
            <HelpSection />
          ) : connected ? (
            id === "auth" ? <ConnectedAuthSection /> : hub.isError ? <QueryError onRetry={() => void hub.refetch()} /> : !hub.data ? (
              <div className="flex flex-col gap-3" aria-busy="true"><Skeleton className="h-16 rounded-2xl" /><Skeleton className="h-80 rounded-2xl" /></div>
            ) : id === "general" ? <GeneralSection settings={hub.data} /> : <ActivitySection settings={hub.data} />
          ) : settings.isError ? (
            <QueryError onRetry={() => void settings.refetch()} />
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
          ) : id === "backups" ? (
            <BackupTargetsSection settings={s} />
          ) : id === "notifications" ? (
            <NotificationsSection settings={s} />
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
