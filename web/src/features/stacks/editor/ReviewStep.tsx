import { AlertTriangle, ArrowLeft, CheckCircle2, CircleX, Download, Info } from "lucide-react";
import { useMemo, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import type { ComposeModel } from "@/domain/compose/model";
import type { Issue } from "@/domain/compose/validate";
import { IconTile } from "@/shared/components/tone";
import { condenseDiff, diffLines } from "@/shared/lib/line-diff";
import { downloadBlob } from "@/shared/lib/tar";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/shared/ui/card";

import { useEditorText } from "./editor-text";

interface ReviewStepProps {
  model: ComposeModel;
  issues: Issue[];
  compose: string;
  savedCompose: string;
  hostName: string;
  onFix: (issue: Issue) => void;
  onBack: () => void;
  deploy: ReactNode;
}

export function ReviewStep({ model, issues, compose, savedCompose, hostName, onFix, onBack, deploy }: ReviewStepProps) {
  const t = useTranslations("editor.review");
  const ts = useTranslations("editor.steps");
  const text = useEditorText();
  const diff = useMemo(() => condenseDiff(diffLines(savedCompose, compose)), [savedCompose, compose]);
  const changed = diff.some((l) => l.kind === "add" || l.kind === "del");
  const tiles = [
    [t("summary.services"), model.services.length],
    [t("summary.ports"), model.services.reduce((n, s) => n + s.ports.length, 0)],
    [t("summary.mounts"), model.services.reduce((n, s) => n + s.mounts.length, 0)],
    [t("summary.host"), hostName],
  ] as const;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2.5">
          <IconTile tone="ok" size="sm">
            <CheckCircle2 />
          </IconTile>
          {t("title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
          {tiles.map(([label, value]) => (
            <div key={label} className="rounded-xl border bg-muted/20 px-3 py-2.5">
              <div className="truncate text-lg font-semibold tabular">{value}</div>
              <div className="text-xs text-muted-foreground">{label}</div>
            </div>
          ))}
        </div>
        {issues.length ? (
          <ul className="flex flex-col divide-y rounded-xl border">
            {issues.map((i, idx) => (
              <li key={idx} className="flex items-center gap-3 px-3 py-2 text-sm">
                {i.level === "error" ? <CircleX className="size-4 shrink-0 text-destructive" aria-hidden /> : <Info className="size-4 shrink-0 text-warning" aria-hidden />}
                <span className="flex-1">
                  {i.service && <span className="font-mono text-xs text-muted-foreground">{i.service}: </span>}
                  {text.issue(i)}
                </span>
                <Button size="sm" variant="outline" onClick={() => onFix(i)}>
                  {t("fix")}
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <Alert className="border-success/30 bg-success/8 text-success">
            <CheckCircle2 />
            <AlertTitle>{t("ok")}</AlertTitle>
          </Alert>
        )}
        <div>
          <div className="mb-1.5 text-xs font-medium text-muted-foreground">{t("diff")}</div>
          {changed ? (
            <pre className="max-h-64 overflow-auto rounded-xl border bg-code p-2 font-mono text-[11.5px] leading-relaxed">
              {diff.map((l, i) =>
                l.kind === "gap" ? (
                  <div key={i} className="px-2 text-muted-foreground/60">
                    ⋯ {l.count}
                  </div>
                ) : (
                  <div key={i} className={cn("px-2", l.kind === "add" && "bg-success/12 text-success", l.kind === "del" && "bg-destructive/12 text-destructive line-through decoration-destructive/40")}>
                    {l.kind === "add" ? "+ " : l.kind === "del" ? "− " : "  "}
                    {l.text}
                  </div>
                ),
              )}
            </pre>
          ) : (
            <p className="text-xs text-muted-foreground">{t("noDiff")}</p>
          )}
        </div>
        <p className="flex items-start gap-2 text-xs text-muted-foreground">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          {t("deployHint")}
        </p>
      </CardContent>
      <CardFooter className="flex-wrap gap-2 border-t">
        <Button size="sm" variant="ghost" onClick={onBack}>
          <ArrowLeft data-icon="inline-start" /> {ts("back")}
        </Button>
        <span className="flex-1" />
        <Button size="sm" variant="outline" onClick={() => downloadBlob("compose.yaml", compose, "text/plain")}>
          <Download data-icon="inline-start" /> compose.yaml
        </Button>
        {deploy}
      </CardFooter>
    </Card>
  );
}
