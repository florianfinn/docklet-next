import { Boxes, HelpCircle, Layers, Server } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import type { ContainerOrigin, Tone } from "@/domain/model";
import { InfoHoverCard } from "@/shared/component-set/info-hover-card";
import { ToneBadge } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";

const META: Record<ContainerOrigin, { tone: Tone; icon: ReactNode }> = {
  compose: { tone: "accent", icon: <Layers /> },
  standalone: { tone: "sky", icon: <Boxes /> },
  unraid: { tone: "orange", icon: <Server /> },
  unknown: { tone: "warn", icon: <HelpCircle /> },
};

/** Origin chip with an explanation of what Docklet may change. */
export function OriginBadge({ origin, note }: { origin: ContainerOrigin; note?: string }) {
  const dt = useAdapterText();
  const t = useTranslations("containers.origin");
  const m = META[origin];
  return (
    <InfoHoverCard
      tone={m.tone}
      icon={m.icon}
      title={t(`${origin}Title`)}
      trigger={
        <ToneBadge tone={m.tone} className="cursor-help">
          {m.icon}
          {t(origin)}
        </ToneBadge>
      }
    >
      <p>{t(`${origin}Body`)}</p>
      {note && <p className="mt-1.5 text-xs">{dt(note)}</p>}
    </InfoHoverCard>
  );
}
