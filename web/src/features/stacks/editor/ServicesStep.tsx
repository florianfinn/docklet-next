import { AlertCircle, ArrowLeft, ArrowRight, Box, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import type { ComposeModel } from "@/domain/compose/model";
import { ops, uniqueKey, type Mutator } from "@/domain/compose/ops";
import type { Issue } from "@/domain/compose/validate";
import { IconTile } from "@/shared/components/tone";
import { useMediaQuery } from "@/shared/hooks/use-media-query";
import { cn } from "@/shared/lib/utils";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/shared/ui/alert-dialog";
import { Button } from "@/shared/ui/button";
import { Card, CardAction, CardContent, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";

import { sectionFor, type SectionKey } from "./sections";
import { EnvSection, ExtrasSection, GeneralSection, HealthSection, MountsSection, NetworksSection, PortsSection } from "./ServiceSections";

interface ServicesStepProps {
  model: ComposeModel;
  issues: Issue[];
  selected: string | null;
  section: SectionKey;
  onSelect: (key: string | null) => void;
  onSection: (s: SectionKey) => void;
  edit: (fn: Mutator) => void;
  onOpenRaw: () => void;
  onBack: () => void;
  onNext: () => void;
}

export function ServicesStep({ model, issues, selected, section, onSelect, onSection, edit, onOpenRaw, onBack, onNext }: ServicesStepProps) {
  const t = useTranslations("editor.services");
  const ts = useTranslations("editor.steps");
  const tc = useTranslations("common.actions");
  const [removing, setRemoving] = useState<string | null>(null);
  // Sections become a vertical list where the wizard has room, wrapping chips elsewhere; never a scrollbar.
  const wide = useMediaQuery("(min-width: 1280px)");
  const current = model.services.find((s) => s.key === selected) ?? model.services[0];
  const svcIssues = issues.filter((i) => i.service === current?.key);

  const addService = () => {
    const key = uniqueKey(
      model.services.map((s) => s.key),
      "service",
    );
    edit(ops.addService(key, { image: "", restart: "unless-stopped" }));
    onSelect(key);
    onSection("general");
  };

  const sections: SectionKey[] = ["general", "ports", "volumes", "env", "health", "networks", "extras"];

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="items-center border-b py-4 [.border-b]:pb-4">
        <CardTitle className="flex items-center gap-2.5">
          <IconTile tone="info" size="sm">
            <Box />
          </IconTile>
          {t("title")}
        </CardTitle>
        <CardAction>
          <Button size="sm" variant="outline" onClick={addService} aria-label={t("addLabel")}>
            <Plus data-icon="inline-start" /> {t("add")}
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="grid gap-0 p-0 md:grid-cols-[15rem_minmax(0,1fr)]">
        <ul className="flex flex-col gap-1 border-b p-2 md:border-r md:border-b-0" aria-label={t("title")}>
          {model.services.length === 0 && <li className="p-3 text-sm text-muted-foreground">{t("empty")}</li>}
          {model.services.map((s) => {
            const errs = issues.filter((x) => x.service === s.key && x.level === "error").length;
            const active = s.key === current?.key;
            return (
              <li key={s.key} className="group flex items-center">
                <button
                  type="button"
                  aria-current={active ? "true" : undefined}
                  onClick={() => onSelect(s.key)}
                  data-glow="row"
                  className={cn(
                    "flex min-w-0 flex-1 flex-col rounded-lg px-2.5 py-2 text-left outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
                    active && "bg-control/15 ring-1 ring-control/50",
                  )}
                >
                  <span className="flex items-center gap-1.5 text-sm font-medium">
                    {s.key}
                    {errs > 0 && <AlertCircle className="size-3.5 text-destructive" aria-label={String(errs)} />}
                  </span>
                  <span className="truncate font-mono text-[11px] text-muted-foreground">{s.image || "—"}</span>
                </button>
                <Button variant="ghost" size="icon" className="size-7 opacity-60 group-hover:opacity-100" onClick={() => setRemoving(s.key)} aria-label={t("remove", { name: s.key })}>
                  <Trash2 />
                </Button>
              </li>
            );
          })}
        </ul>
        <div className="min-w-0 p-4 md:p-5">
          {current ? (
            <Tabs value={section} onValueChange={(v) => onSection(v as SectionKey)} orientation={wide ? "vertical" : "horizontal"} className={cn(wide && "flex-row items-start gap-6")}>
              <TabsList
                aria-label={t("sectionsLabel", { name: current.key })}
                data-slot="service-sections"
                className={cn(
                  "w-full flex-wrap justify-start gap-1 p-1 group-data-[orientation=horizontal]/tabs:mb-3 group-data-[orientation=horizontal]/tabs:h-auto",
                  wide && "sticky top-20 w-52 shrink-0 items-stretch",
                )}
              >
                {sections.map((k) => {
                  const err = svcIssues.some((i) => sectionFor(i) === k && i.level === "error");
                  const count =
                    k === "ports" ? current.ports.length : k === "volumes" ? current.mounts.length : k === "env" ? current.env.length : k === "extras" ? current.extras.length : undefined;
                  return (
                    <TabsTrigger
                      key={k}
                      value={k}
                      // Selected section in the control colour of the design (as segmented toggles), not the neutral tab style.
                      className="h-9 flex-none gap-2 px-3 text-sm data-[state=active]:border-control/55 data-[state=active]:bg-control/15 data-[state=active]:text-control-text dark:data-[state=active]:border-control/55 dark:data-[state=active]:bg-control/15 dark:data-[state=active]:text-control-text"
                    >
                      {t(`sections.${k}`)}
                      {count ? <span className="rounded bg-muted px-1.5 text-[11px] tabular">{count}</span> : null}
                      {err && <AlertCircle className="size-3.5 text-destructive" aria-hidden />}
                    </TabsTrigger>
                  );
                })}
              </TabsList>
              {(
                [
                  ["general", GeneralSection],
                  ["ports", PortsSection],
                  ["volumes", MountsSection],
                  ["env", EnvSection],
                  ["health", HealthSection],
                  ["networks", NetworksSection],
                ] as const
              ).map(([k, Section]) => (
                <TabsContent key={k} value={k} className="min-w-0">
                  <Section
                    service={current}
                    issues={svcIssues}
                    edit={edit}
                    onRename={(to) => {
                      const clean = to.trim();
                      if (!clean || clean === current.key) return;
                      edit(ops.renameService(current.key, clean));
                      onSelect(clean);
                    }}
                  />
                </TabsContent>
              ))}
              <TabsContent value="extras" className="min-w-0">
                <ExtrasSection service={current} issues={svcIssues} edit={edit} onOpenRaw={onOpenRaw} />
              </TabsContent>
            </Tabs>
          ) : (
            <p className="text-sm text-muted-foreground">{t("empty")}</p>
          )}
        </div>
      </CardContent>
      <CardFooter className="justify-between border-t py-3">
        <Button size="sm" variant="ghost" onClick={onBack}>
          <ArrowLeft data-icon="inline-start" /> {ts("back")}
        </Button>
        <Button size="sm" onClick={onNext} className="group">
          {ts("next")}: {ts("review")}
          <ArrowRight data-icon="inline-end" className="nudge" />
        </Button>
      </CardFooter>
      <AlertDialog open={!!removing} onOpenChange={(o) => !o && setRemoving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("removeConfirm", { name: removing ?? "" })}</AlertDialogTitle>
            <AlertDialogDescription>{t("removeBody")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{tc("cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (removing) edit(ops.removeService(removing));
                onSelect(null);
                setRemoving(null);
              }}
            >
              {t("removeAction")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
