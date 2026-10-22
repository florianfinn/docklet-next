import { useCapability } from "@/data/use-capability";
import { CheckCircle2, CircleAlert, Container, GitBranch, Import } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import type { ImportRequest, ImportResult } from "@/data/adapter";
import { useAdapter } from "@/data/use-adapter";
import { ToneBadge } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Spinner } from "@/shared/ui/spinner";
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs";

const EXAMPLES: Record<ImportRequest["kind"], string[]> = {
  github: ["github.com/example/notes-app", "github.com/example/tool"],
  dockerhub: ["traefik/whoami", "linuxserver/syncthing"],
};

interface ImportSourceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onApply: (result: ImportResult) => void;
}

/** One-time import through the adapter. */
export function ImportSourceDialog({ open, onOpenChange, onApply }: ImportSourceDialogProps) {
  const t = useTranslations("editor.source");
  const tc = useTranslations("common.actions");
  const dt = useAdapterText();
  const adapter = useAdapter();
  const capability = useCapability("importSource");
  const [kind, setKind] = useState<ImportRequest["kind"]>("github");
  const [ref, setRef] = useState("");
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [error, setError] = useState<"missing" | "unavailable" | null>(null);

  const fetchSource = async (value = ref) => {
    if (!capability.available) return;
    if (!value.trim()) {
      setError("missing");
      return;
    }
    setError(null);
    setLoading(true);
    try {
      setResult(await adapter.importSource({ kind, ref: value }));
    } catch {
      setResult(null);
      setError("unavailable");
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) setResult(null);
      }}
    >
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Import className="size-4 text-primary-text" /> {t("title")}
          </DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        {!capability.available && <p role="status" className="text-sm text-muted-foreground">{capability.reason}</p>}
        <Tabs
          value={kind}
          onValueChange={(v) => {
            setKind(v as ImportRequest["kind"]);
            setResult(null);
            setRef("");
          }}
        >
          <TabsList>
            <TabsTrigger value="github" className="gap-1.5">
              <GitBranch /> {t("github")}
            </TabsTrigger>
            <TabsTrigger value="dockerhub" className="gap-1.5">
              <Container /> {t("dockerhub")}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void fetchSource();
          }}
        >
          <Field data-invalid={Boolean(error) || undefined}>
            <FieldLabel htmlFor="import-ref">{t("ref")}</FieldLabel>
            <div className="flex gap-2">
              <Input id="import-ref" className="font-mono" aria-invalid={Boolean(error) || undefined} placeholder={kind === "github" ? t("githubPlaceholder") : t("dockerhubPlaceholder")} value={ref} onChange={(e) => setRef(e.target.value)} />
              <Button type="submit" disabled={loading || !capability.available} title={capability.reason}>
                {loading && <Spinner data-icon="inline-start" />}
                {t("fetch")}
              </Button>
            </div>
            <FieldDescription className={error ? "text-destructive" : undefined}>{error === "missing" ? t("refMissing") : error === "unavailable" ? t("fetchFailed") : t("simulated")}</FieldDescription>
          </Field>
          <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            {t("examples")}:
            {EXAMPLES[kind].map((ex) => (
              <button
                key={ex}
                disabled={!capability.available}
                title={capability.reason}
                type="button"
                className="rounded-md border border-dashed px-2 py-0.5 font-mono hover:border-primary/50 hover:text-foreground"
                onClick={() => {
                  setRef(ex);
                  void fetchSource(ex);
                }}
              >
                {ex}
              </button>
            ))}
          </div>
        </form>
        {result && (
          <div className="flex flex-col gap-3 rounded-xl border bg-muted/20 p-3">
            <div className="flex items-center gap-2 text-sm font-medium">
              {dt(result.title)}
              {import.meta.env.DEV && adapter.kind === "demo" && <ToneBadge tone="violet">Simulation</ToneBadge>}
            </div>
            <div className="grid gap-3 md:grid-cols-2">
              <div>
                <div className="mb-1 text-xs font-medium text-success">{t("found")}</div>
                <ul className="flex flex-col gap-1 text-xs">
                  {result.found.map((f) => (
                    <li key={f} className="flex gap-1.5">
                      <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-success" aria-hidden /> {dt(f)}
                    </li>
                  ))}
                </ul>
              </div>
              <div>
                <div className="mb-1 text-xs font-medium text-warning">{t("missing")}</div>
                <ul className="flex flex-col gap-1 text-xs">
                  {result.missing.map((f) => (
                    <li key={f} className="flex gap-1.5">
                      <CircleAlert className="mt-0.5 size-3.5 shrink-0 text-warning" aria-hidden /> {dt(f)}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {tc("cancel")}
          </Button>
          <Button
            disabled={!result}
            onClick={() => {
              if (!result) return;
              onApply(result);
              onOpenChange(false);
              setResult(null);
              setRef("");
            }}
          >
            {t("apply")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
