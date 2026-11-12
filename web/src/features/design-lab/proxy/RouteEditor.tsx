import { useState } from "react";
import { ArrowDown, ArrowLeft, ArrowRight, Code2, Globe, Save, Wand2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { FieldGroup } from "@/shared/ui/field";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Textarea } from "@/shared/ui/textarea";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { cn } from "@/shared/lib/utils";
import { FormSection } from "./controls";
import { useProxyDesign } from "./design";
import { AddressFields, BehaviorFields, TargetFields } from "./RouteFields";
import { RouteTlsFields } from "./RouteTlsFields";
import { routeName, routeNameConflict, validRouteName, wildcardProblem, routeFromYaml, routeYaml, upsertRoute, useProxy, validRoute, type ProxyRoute } from "./model";
import { useProxyText } from "./use-proxy-text";
export type EditorStep = "address" | "destination" | "tls" | "behavior";
const STEPS: EditorStep[] = ["address", "destination", "tls", "behavior"];
const STEP_LABELS = { address: "stepAddress", destination: "stepDestination", tls: "stepTls", behavior: "stepBehavior" } as const;

export function RouteEditor({ initial, creating = false, onClose, initialStep = "address" }: { initial: ProxyRoute; creating?: boolean; onClose: () => void; initialStep?: EditorStep }) {
  const t = useProxyText();
  const design = useProxyDesign();
  const { resolvers, routes } = useProxy();
  const [route, setRoute] = useState(initial);
  const [mode, setMode] = useState("form");
  const [section, setSection] = useState(design === "manager" ? initialStep === "tls" ? "tls" : "details" : initialStep);
  const [yaml, setYaml] = useState(() => routeYaml(initial));
  const [error, setError] = useState("");
  const [kind, setKind] = useState(initial.containerId ? "container" : "external");
  const [additional, setAdditional] = useState(initial.backends.join("\n"));
  const patch = (data: Partial<ProxyRoute>) => setRoute((r) => ({ ...r, ...data }));
  function changeMode(next: string) {
    try {
      const currentYaml = routeYaml({ ...route, backends: additional.split("\n").map((s) => s.trim()).filter(Boolean) });
      if (next === "yaml") setYaml(currentYaml);
      // An unchanged generated draft may still be incomplete; switching views must not validate it.
      else if (yaml !== currentYaml) { const parsed = routeFromYaml(yaml, route); setRoute(parsed); setAdditional(parsed.backends.join("\n")); setKind(parsed.containerId ? "container" : "external"); }
      setError(""); setMode(next);
    } catch { setError(t("invalidYaml")); }
  }
  function save() {
    try {
      const candidate = mode === "yaml" ? routeFromYaml(yaml, route) : { ...route, domain: route.domain.trim().toLowerCase(), backends: additional.split("\n").map((s) => s.trim()).filter(Boolean) };
      if (!validRouteName(routeName(candidate))) { setError(t("routeNameInvalid")); return; }
      if (routeNameConflict(candidate, routes)) { setError(t("routeNameTaken")); return; }
      if (!validRoute(candidate)) { setError(t("validation")); return; }
      const problem = wildcardProblem(candidate, resolvers);
      if (problem) { setError(t(problem)); return; }
      upsertRoute(candidate); toast.success(t("saved")); onClose();
    } catch { setError(t("invalidYaml")); }
  }
  const address = <AddressFields route={route} patch={patch} showPath={design !== "manager"} />;
  const destination = <TargetFields route={route} patch={patch} kind={kind} setKind={setKind} additional={additional} setAdditional={setAdditional} split={design === "manager"} />;
  const tls = <FormSection title={t("tlsGroup")} tone="tls"><RouteTlsFields route={route} patch={patch} /></FormSection>;
  const behavior = <BehaviorFields route={route} patch={patch} showPath={design === "manager"} />;
  const step = STEPS.indexOf(section as EditorStep);
  const sections = design === "manager"
    ? [{ value: "details", label: t("hostDetails"), hint: t("detailsStepHint") }, { value: "behavior", label: t("routingRules"), hint: t("optionsStepHint") }, { value: "tls", label: t("stepTls"), hint: t("sslStepHint") }]
    : STEPS.map((value) => ({ value, label: t(STEP_LABELS[value]), hint: t(({ address: "addressStepHint", destination: "targetStepHint", tls: "sslStepHint", behavior: "optionsStepHint" } as const)[value]) }));
  return <Dialog open onOpenChange={(open) => !open && onClose()}><DialogContent className="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
    <DialogHeader className="border-b p-5 pr-12 text-left"><DialogTitle>{t(creating ? "newRoute" : "editRoute")}</DialogTitle><DialogDescription>{t("sharedDraftHint")}</DialogDescription>
      <div className="pt-3"><SegmentedToggle value={mode} onChange={changeMode} label={t("editorMode")} options={[{ value: "form", label: t("easyMode"), icon: <Wand2 />, hint: t("easyModeHint") }, { value: "yaml", label: t("advancedMode"), icon: <Code2 />, hint: t("advancedModeHint") }]} /></div>

    </DialogHeader>
    <form id="proxy-route-form" className="min-h-0 flex-1 overflow-y-auto p-5" onSubmit={(e) => { e.preventDefault(); save(); }}>
      {mode === "form" && design !== "classic" && <ol aria-label={t("routeSteps")} className={cn("mb-5 grid gap-2 sm:grid-cols-3", design === "workspace" && "sm:grid-cols-4")}>
        {sections.map((item, index) => {
          const active = section === item.value;
          return <li key={item.value}><button type="button" aria-current={active ? "step" : undefined} onClick={() => setSection(item.value)} data-glow="row" className={cn("lift flex h-full w-full items-center gap-2.5 rounded-xl border bg-card px-3 py-2.5 text-left text-muted-foreground outline-none hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50", active && "border-control/55 bg-control/15 text-foreground")}>
            <span className={cn("grid size-7 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold transition-colors duration-(--dur)", active && "bg-control text-control-foreground")}>{index + 1}</span>
            <span className="flex min-w-0 flex-col gap-0.5 leading-tight"><strong className="text-sm font-medium">{item.label}</strong><small className="text-xs text-muted-foreground">{item.hint}</small></span>
          </button></li>;
        })}
      </ol>}
      {mode === "yaml" ? <Textarea aria-label={t("yaml")} value={yaml} onChange={(e) => setYaml(e.target.value)} className="min-h-100 font-mono text-xs" spellCheck={false} /> : design === "classic" ? <FieldGroup className="grid items-start gap-5 md:grid-cols-2">{address}{destination}{tls}{behavior}</FieldGroup> : design === "manager" ? <FieldGroup className="gap-5">{section === "details" && <>{address}{destination}</>}{section === "tls" && tls}{section === "behavior" && behavior}</FieldGroup> : <div className="grid items-start gap-5 md:grid-cols-[minmax(0,1fr)_14rem]"><FieldGroup>{section === "address" && address}{section === "destination" && destination}{section === "tls" && tls}{section === "behavior" && behavior}</FieldGroup><aside className="flex flex-col items-start gap-3 rounded-xl border bg-muted/20 p-4 text-sm"><span className="text-xs text-muted-foreground">{t("routeSummary")}</span><Globe className="size-5 text-cyan" /><strong className="break-all">{route.domain || t("unconfiguredDomain")}</strong><ArrowDown className="size-4 text-muted-foreground" /><span>{t(route.tls === "off" ? "noSsl" : route.tls === "auto" ? "requestSsl" : "existingSsl")}</span><ArrowDown className="size-4 text-muted-foreground" /><code className="break-all text-xs">{route.url || t("unconfiguredTarget")}</code></aside></div>}
      {error && <p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}
    </form>
    <DialogFooter className="flex-row flex-wrap items-center justify-end border-t p-4"><Button variant="outline" onClick={onClose}>{t("cancel")}</Button>{design === "workspace" && mode === "form" && <>{step > 0 && <Button variant="ghost" onClick={() => setSection(STEPS[step - 1]!)}><ArrowLeft data-icon="inline-start" />{t("previousStep")}</Button>}{step < 3 && <Button variant="outline" onClick={() => setSection(STEPS[step + 1]!)}>{t("nextStep")}<ArrowRight data-icon="inline-end" /></Button>}</>}<Button type="submit" form="proxy-route-form"><Save data-icon="inline-start" />{t("save")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
