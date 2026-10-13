import { ArrowLeft, CircleDashed } from "lucide-react";
import { Link, Navigate, useParams } from "react-router";
import { useMessages, useTranslations } from "use-intl";

import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";

import { PREVIEW_ICONS } from "../shell/nav-items";
import { dePreview } from "./messages/de";

type Section = keyof typeof dePreview.sections;

/** Sections that left the preview stage; old links forward to the real pages. */
const MOVED: Record<string, string> = {
  hosts: "/hosts",
  images: "/images",
  volumes: "/volumes",
  backups: "/backups",
  notifications: "/settings/notifications",
  settings: "/settings",
  profile: "/profile",
};

/** Honest placeholder for sections outside phase one: scope only, no dead controls. */
export default function PreviewPage() {
  const { section } = useParams();
  const t = useTranslations("preview");
  const sections = useMessages().preview.sections;
  if (section && MOVED[section]) return <Navigate to={MOVED[section]} replace />;
  if (!section || !(section in dePreview.sections)) return <Navigate to="/dashboard" replace />;
  const key = section as Section;
  const Icon = PREVIEW_ICONS[key] ?? CircleDashed;
  const points = sections[key].points;
  return (
    <div className="mx-auto max-w-2xl pt-6">
      <Card className="overflow-hidden">
        <CardHeader className="grid-cols-[auto_1fr] items-center gap-x-4">
          <IconTile tone="accent" size="lg">
            <Icon />
          </IconTile>
          <div>
            <ToneBadge tone="violet" className="mb-1">
              {t("badge")}
            </ToneBadge>
            <CardTitle className="text-xl">{t(`sections.${key}.title`)}</CardTitle>
            <CardDescription>{t("lead")}</CardDescription>
          </div>
        </CardHeader>
        <CardContent>
          <ul className="flex flex-col gap-2">
            {points.map((p) => (
              <li key={p} className="flex gap-2.5 rounded-lg border bg-muted/30 p-3 text-sm">
                <CircleDashed className="mt-0.5 size-4 shrink-0 text-primary-text" aria-hidden />
                {p}
              </li>
            ))}
          </ul>
          <p className="mt-4 text-xs text-muted-foreground">{t("noFake")}</p>
        </CardContent>
        <CardFooter>
          <Button asChild variant="outline">
            <Link to="/dashboard">
              <ArrowLeft data-icon="inline-start" />
              {t("back")}
            </Link>
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
}
