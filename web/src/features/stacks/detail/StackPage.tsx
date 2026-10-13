import { ArrowLeft, FileCode2, FileText, History, LayoutGrid, Layers } from "lucide-react";
import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { useStack, useStacks } from "@/data/queries";
import type { StackDetail } from "@/domain/model";
import { EmptyState, QueryError } from "@/shared/components/page";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Button } from "@/shared/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";

import { NEW_STACK_DRAFT } from "../editor/new-stack-draft";
import { StackWorkspace, type EditorMode } from "../editor/StackWorkspace";
import { useComposeDraft } from "../editor/use-compose-draft";
import { StackLogs } from "../logs/StackLogs";
import { allLabels } from "../overview/stack-filters";
import { DeploymentsTab } from "./DeploymentsTab";
import { OverviewTab } from "./OverviewTab";
import { StackHeader } from "./StackHeader";
import { StackPageSkeleton } from "./StackPageSkeleton";

type Tab = "overview" | "editor" | "logs" | "deployments";
const TABS: { id: Tab; icon: typeof LayoutGrid }[] = [
  { id: "overview", icon: LayoutGrid },
  { id: "editor", icon: FileCode2 },
  { id: "logs", icon: FileText },
  { id: "deployments", icon: History },
];

function BackLink() {
  const t = useTranslations("stacks.detail");
  return (
    <Button asChild variant="ghost" size="sm" className="group mb-3 -ml-2 text-muted-foreground">
      <Link to="/stacks">
        <ArrowLeft data-icon="inline-start" className="transition-transform duration-(--dur) group-hover:-translate-x-0.5" />
        {t("back")}
      </Link>
    </Button>
  );
}

/** Existing stack: header, tabs and the shared draft kept across tab switches. */
function ExistingStack({ stack }: { stack: StackDetail }) {
  const t = useTranslations("stacks.detail");
  const { byId, all } = useHostScope();
  const stacks = useStacks();
  const [params, setParams] = useSearchParams();
  const tab = (TABS.some((x) => x.id === params.get("tab")) ? params.get("tab") : "overview") as Tab;
  const [mode, setMode] = useState<EditorMode>("advanced");
  const draft = useComposeDraft({ compose: stack.compose, env: stack.env, hostId: stack.hostId, labels: stack.labels });
  const host = byId.get(stack.hostId);

  return (
    <>
      <StackHeader stack={stack} host={host} inline={tab === "deployments"} />
      <Tabs value={tab} onValueChange={(v) => setParams((p) => ({ ...Object.fromEntries(p), tab: v }), { replace: true })}>
        <TabsList aria-label={t("tabsLabel")} className="mb-4 max-w-full flex-wrap justify-start group-data-[orientation=horizontal]/tabs:h-auto">
          {TABS.map(({ id, icon: Icon }) => (
            <TabsTrigger key={id} value={id} className="gap-1.5 px-3">
              <Icon /> {t(`tabs.${id}`)}
              {id === "editor" && draft.dirty && <i className="size-1.5 rounded-full bg-primary" aria-hidden />}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="overview">
          <OverviewTab stack={stack} host={host} />
        </TabsContent>
        <TabsContent value="editor">
          <StackWorkspace draft={draft} stack={stack} hosts={all} labelSuggestions={allLabels(stacks.data ?? [])} mode={mode} onMode={setMode} />
        </TabsContent>
        <TabsContent value="logs">
          <StackLogs stack={stack} />
        </TabsContent>
        <TabsContent value="deployments">
          <DeploymentsTab stack={stack} />
        </TabsContent>
      </Tabs>
    </>
  );
}

function NewStack() {
  const t = useTranslations("stacks.detail");
  const tl = useTranslations("locale.ui");
  const { all, visible } = useHostScope();
  const stacks = useStacks();
  const [params] = useSearchParams();
  const requested = all.find((h) => h.id === params.get("host") && h.status === "online")?.id;
  const firstOnline = requested ?? (visible.find((h) => h.status === "online") ?? all.find((h) => h.status === "online"))?.id ?? "atlas";
  const [mode, setMode] = useState<EditorMode>("easy");
  const draft = useComposeDraft({ compose: NEW_STACK_DRAFT, env: "TZ=Europe/Berlin\n", hostId: firstOnline, labels: [] });
  const host = all.find((h) => h.id === draft.hostId);
  const name = draft.parsed.ok ? draft.parsed.model.name : "";
  return (
    <>
      <header className="mb-5 flex flex-wrap items-start gap-4">
        <IconTile color={host?.color} size="lg">
          <Layers />
        </IconTile>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{name || tl("newStack")}</h1>
            <ToneBadge tone="accent">{t("draft")}</ToneBadge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">{t("draftMeta", { host: host?.name ?? "" })}</p>
        </div>
      </header>
      <StackWorkspace draft={draft} stack={null} hosts={all} labelSuggestions={allLabels(stacks.data ?? [])} mode={mode} onMode={setMode} />
    </>
  );
}

export default function StackPage() {
  const { stackId } = useParams();
  const t = useTranslations("stacks.detail");
  const isNew = !stackId;
  const stack = useStack(isNew ? undefined : stackId);
  const hosts = useHostScope();

  if (isNew) {
    return (
      <>
        <BackLink />
        {hosts.isPending ? <StackPageSkeleton /> : hosts.isError ? <QueryError onRetry={() => void hosts.refetch()} /> : <NewStack />}
      </>
    );
  }
  return (
    <>
      <BackLink />
      {stack.isPending || hosts.isPending ? (
        <StackPageSkeleton />
      ) : hosts.isError ? (
        <QueryError onRetry={() => void hosts.refetch()} />
      ) : stack.isError ? (
        stack.error.message === "not-found" ? (
          <EmptyState icon={<Layers />} title={t("notFound")} description={t("notFoundBody")} />
        ) : (
          <QueryError onRetry={() => void stack.refetch()} />
        )
      ) : (
        <ExistingStack key={stack.data.id} stack={stack.data} />
      )}
    </>
  );
}
