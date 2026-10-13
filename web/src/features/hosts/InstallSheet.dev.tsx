import { useEffect, useState } from "react";
import { useTranslations } from "use-intl";

import { useSettings } from "@/data/queries";
import { agentHubUrl } from "@/domain/settings";
import type { Host } from "@/domain/model";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Skeleton } from "@/shared/ui/skeleton";

import { InstallArtifacts } from "./InstallArtifacts";
import { planFromDraft, type HostDraft } from "./onboarding/onboarding-draft";
import { useHostCommands } from "./use-host-commands";

function draftOf(host: Host, token: string): HostDraft {
  const [address = "", port = "3552"] = host.address.split(":");
  return {
    name: host.name,
    color: host.color,
    connection: host.connection,
    address,
    port,
    projectDir: host.projectDir,
    installDir: host.installDir,
    manualIds: !!host.fileOwner,
    puid: host.fileOwner ? String(host.fileOwner.uid) : "",
    pgid: host.fileOwner ? String(host.fileOwner.gid) : "",
    detectSocket: true,
    socketGid: "",
    token,
    simulateFailure: false,
    reached: 0,
  };
}

/** Regenerated install files for an existing host with a fresh demo token. */
export default function InstallSheet({ host, onClose }: { host: Host | null; onClose: () => void }) {
  const t = useTranslations("hosts.install");
  const settings = useSettings();
  const cmd = useHostCommands();
  const [token, setToken] = useState<string | null>(null);
  const hostName = host?.name;
  useEffect(() => {
    if (!hostName) return;
    let live = true;
    void cmd.token(hostName).then((tk) => live && setToken(tk ?? null));
    return () => {
      live = false;
      setToken(null);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- refresh only per host
  }, [hostName]);
  const general = settings.data?.general;
  return (
    <Sheet open={!!host} onOpenChange={(o) => !o && onClose()}>
      <SheetContent className="w-full gap-0 p-0 sm:max-w-2xl">
        {host && (
          <>
            <SheetHeader className="border-b">
              <SheetTitle>{t("sheetTitle", { name: host.name })}</SheetTitle>
              <SheetDescription>{t("sheetDescription")}</SheetDescription>
            </SheetHeader>
            <ScrollArea className="min-h-0 flex-1">
              <div className="p-4">
                {token && general ? (
                  <InstallArtifacts plan={planFromDraft(draftOf(host, token), agentHubUrl(general), general.timeZone)} onRegenerateToken={() => void cmd.token(host.name).then((tk) => tk && setToken(tk))} />
                ) : (
                  <Skeleton className="h-80 rounded-xl" />
                )}
              </div>
            </ScrollArea>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
