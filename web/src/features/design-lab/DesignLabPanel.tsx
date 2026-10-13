import { useQueryClient } from "@tanstack/react-query";
import { FlaskConical, RotateCcw } from "lucide-react";
import { useEffect } from "react";
import { useTranslations } from "use-intl";

import { simulationStore } from "@/data/demo/simulation";
import { keepOpenForCoordinatedLayers } from "@/features/activity/layer-coordination";
import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Kbd } from "@/shared/ui/kbd";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { ControlsSection, FlowsSection } from "./ControlsSection";
import { designStore, PRESETS, type DesignTokens } from "@/shared/theme/design-tokens";
import { FavoritesSection } from "@/shared/preferences/FavoritesSection";
import { LayoutSection, LookSection, MotionSection, SimulationSection } from "./LabSections";
import { labOpenStore } from "@/features/design-lab/lab-open-store";

const STORAGE_PREFIX = "docklet-react-opus.";

function applyPreset(tokens: DesignTokens) {
  designStore.set((s) => ({ ...s, tokens, active: null }));
}

function toggleSlots() {
  designStore.set((s) => {
    const next = s.active === "A" ? "B" : "A";
    const tokens = next === "A" ? s.slotA : s.slotB;
    return tokens ? { ...s, tokens, active: next } : s;
  });
}

function PresetsSection() {
  const t = useTranslations("lab.presets");
  const state = useStore(designStore, (s) => s);
  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">{t("hint")}</p>
      <ul className="flex flex-col gap-2">
        {PRESETS.map((p) => {
          const active = JSON.stringify(p.tokens) === JSON.stringify(state.tokens);
          return (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => applyPreset(p.tokens)}
                aria-pressed={active}
                className={cn(
                  "lift group flex w-full items-center gap-3 rounded-xl border bg-card/60 p-3 text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
                  active && "border-primary/60 bg-primary/10",
                )}
              >
                <span className="flex shrink-0 -space-x-1.5" aria-hidden>
                  <i className="size-5 rounded-full border-2 border-card" style={{ background: p.tokens.palette.primary }} />
                  <i className="size-5 rounded-full border-2 border-card" style={{ background: p.tokens.palette.secondary }} />
                  <i className="size-5 rounded-full border-2 border-card" style={{ background: `oklch(0.77 ${0.17 * p.tokens.semantic} 163)` }} />
                  <i className="size-5 rounded-full border-2 border-card" style={{ background: p.tokens.theme === "dark" ? "oklch(0.2 0 0)" : "oklch(0.98 0 0)" }} />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-medium">{t(`names.${p.id}` as "names.arcane")}</span>
                  <span className="block text-xs text-muted-foreground">{t(`descriptions.${p.id}` as "descriptions.arcane")}</span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      <div className="grid grid-cols-2 gap-2">
        <Button variant="outline" size="sm" onClick={() => designStore.set((s) => ({ ...s, slotA: s.tokens, active: "A" }))}>
          {t("slotA")}
        </Button>
        <Button variant="outline" size="sm" onClick={() => designStore.set((s) => ({ ...s, slotB: s.tokens, active: "B" }))}>
          {t("slotB")}
        </Button>
        <Button variant="secondary" size="sm" className="col-span-2" disabled={!state.slotA || !state.slotB} onClick={toggleSlots}>
          {t("toggle")} <Kbd>Alt ⇧ C</Kbd>
        </Button>
        <p className="col-span-2 text-center text-xs text-muted-foreground">{t("active", { slot: state.active ?? "–" })}</p>
      </div>
      <FavoritesSection />
      <Button
        variant="ghost"
        className="text-destructive hover:text-destructive"
        onClick={() => {
          Object.keys(localStorage)
            .filter((k) => k.startsWith(STORAGE_PREFIX))
            .forEach((k) => localStorage.removeItem(k));
          window.location.reload();
        }}
      >
        <RotateCcw data-icon="inline-start" />
        {t("reset")}
      </Button>
      <p className="-mt-3 text-center text-[11px] text-muted-foreground">{t("resetHint")}</p>
    </div>
  );
}

/** Developer menu: design tokens, motion, density, simulation and presets. */
export function DesignLabPanel() {
  const t = useTranslations("lab");
  const open = useStore(labOpenStore, (o) => o);
  const client = useQueryClient();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.altKey || !e.shiftKey) return;
      if (e.code === "KeyD") {
        e.preventDefault();
        labOpenStore.set((o) => !o);
      } else if (e.code === "KeyC") {
        e.preventDefault();
        toggleSlots();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    // Replays loading only when a read-affecting knob changes, not for live/action toggles.
    let prev = simulationStore.get();
    return simulationStore.subscribe(() => {
      const next = simulationStore.get();
      if (next.latency !== prev.latency || next.failArea !== prev.failArea || next.empty !== prev.empty) void client.resetQueries();
      prev = next;
    });
  }, [client]);

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="icon"
            className="lift fixed right-5 bottom-5 z-40 size-11 rounded-full shadow-lg shadow-highlight/30"
            onClick={() => labOpenStore.set(true)}
            aria-label={t("open")}
          >
            <FlaskConical className="tilt" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">
          {t("open")} <Kbd>{t("shortcut")}</Kbd>
        </TooltipContent>
      </Tooltip>
      {/* Non-modal, so toasts, the action dialog and the page stay usable while comparing designs. */}
      <Sheet modal={false} open={open} onOpenChange={(o) => labOpenStore.set(o)}>
        <SheetContent className="w-full gap-0 p-0 shadow-2xl sm:max-w-md" data-layer="design-lab" onInteractOutside={keepOpenForCoordinatedLayers}>
          <SheetHeader className="border-b">
            <SheetTitle className="flex items-center gap-2">
              <FlaskConical className="size-4 text-primary-text" />
              {t("title")}
            </SheetTitle>
            <SheetDescription>{t("description")}</SheetDescription>
          </SheetHeader>
          <Tabs defaultValue="look" className="min-h-0 flex-1 gap-0">
            <TabsList className="mx-4 mt-3 grid w-auto grid-cols-6">
              {(["look", "motion", "layout", "controls", "simulation", "presets"] as const).map((k) => (
                <TabsTrigger key={k} value={k} className="px-1 text-xs">
                  {t(`tabs.${k}`)}
                </TabsTrigger>
              ))}
            </TabsList>
            <ScrollArea className="min-h-0 flex-1">
              <div className="p-4">
                <TabsContent value="look">
                  <LookSection />
                </TabsContent>
                <TabsContent value="motion">
                  <MotionSection />
                </TabsContent>
                <TabsContent value="layout">
                  <LayoutSection />
                </TabsContent>
                <TabsContent value="controls">
                  <ControlsSection />
                </TabsContent>
                <TabsContent value="simulation" className="flex flex-col gap-6">
                  <SimulationSection onReload={() => void client.resetQueries()} />
                  <FlowsSection />
                </TabsContent>
                <TabsContent value="presets">
                  <PresetsSection />
                </TabsContent>
              </div>
            </ScrollArea>
          </Tabs>
        </SheetContent>
      </Sheet>
    </>
  );
}
