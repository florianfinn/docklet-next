// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/features/stacks/detail/StackPage.tsx
// Revision: d6c87ce35acb0a517963f61e62852640a9dce5d99a34d2a2575077660b53bae0
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Guard rejected host queries before mounting the stack editor; preserves the historical L4 import and existing UI catalogue values. See web/PROVENANCE.md. #82 host inventory observations, localized stale notices and read-only controls. #27 production text, source errors and capability boundaries. M14 #318 host maintenance and availability web implementation. Simple-user management, assigned-stack access and mode-aware self-service.

import { isHostAvailable } from "@/domain/hosts";
import { InventoryNotice } from "../InventoryNotice";
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

import { useIsUser } from "@/data/use-principal";
import { UserStack } from "./UserStack";
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

/** Shown instead of the editor while compose.yaml and .env cannot be read. */
function FilesUnavailable({ onRetry }: { onRetry: () => void }) {
  const t = useTranslations("editor");
  const tc = useTranslations("common.actions");
  return (
    <div role="alert" className="flex flex-wrap items-center gap-3 rounded-lg border border-warning/30 bg-warning/8 px-3 py-2 text-sm text-warning">
      <span className="flex-1">{t("filesUnavailable")}</span>
      <Button size="sm" variant="outline" onClick={onRetry}>{tc("retry")}</Button>
    </div>
  );
}

/** Existing stack: header, tabs and the shared draft kept across tab switches. */
function ExistingStack({ stack, onRetry }: { stack: StackDetail; onRetry: () => void }) {
  const t = useTranslations("stacks.detail");
  const { byId, all } = useHostScope();
  const stacks = useStacks();
  const [params, setParams] = useSearchParams();
  const tab = (TABS.some((x) => x.id === params.get("tab")) ? params.get("tab") : "overview") as Tab;
  const [mode, setMode] = useState<EditorMode>("advanced");
  // The draft only ever starts from files that were actually read. A later read
  // failure keeps the last loaded files as its base; writes stay gated meanwhile.
  const [files, setFiles] = useState(stack.filesUnavailable ? undefined : { compose: stack.compose, env: stack.env });
  if (!stack.filesUnavailable && (files?.compose !== stack.compose || files?.env !== stack.env)) setFiles({ compose: stack.compose, env: stack.env });
  const draft = useComposeDraft({ compose: files?.compose ?? "", env: files?.env ?? "", hostId: stack.hostId, labels: stack.labels });
  const host = byId.get(stack.hostId);

  return (
    <>
      <InventoryNotice hosts={host ? [host] : []} items={[stack]} />
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
          {!files ? (
            <FilesUnavailable onRetry={onRetry} />
          ) : (
            <StackWorkspace draft={draft} stack={stack} hosts={all} labelSuggestions={allLabels(stacks.data ?? [])} mode={mode} onMode={setMode} />
          )}
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
  const requested = all.find((h) => h.id === params.get("host") && isHostAvailable(h))?.id;
  const firstOnline = requested ?? (visible.find((h) => isHostAvailable(h)) ?? all.find((h) => isHostAvailable(h)))?.id ?? "atlas";
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
  const user = useIsUser();
  const isNew = !stackId;
  const stack = useStack(isNew ? undefined : stackId);
  const hosts = useHostScope();

  if (isNew) {
    return (
      <>
        <BackLink />
        {hosts.isPending ? <StackPageSkeleton /> : hosts.isError ? <QueryError error={hosts.error} onRetry={() => void hosts.refetch()} /> : <NewStack />}
      </>
    );
  }
  return (
    <>
      <BackLink />
      {stack.isPending || hosts.isPending ? (
        <StackPageSkeleton />
      ) : hosts.isError ? (
        <QueryError error={hosts.error} onRetry={() => void hosts.refetch()} />
      ) : stack.isError ? (
        stack.error.message === "not-found" ? (
          <EmptyState icon={<Layers />} title={t("notFound")} description={t("notFoundBody")} />
        ) : (
          <QueryError error={stack.error} onRetry={() => void stack.refetch()} />
        )
      ) : (
        user ? <UserStack key={stack.data.id} stack={stack.data} /> : <ExistingStack key={stack.data.id} stack={stack.data} onRetry={() => void stack.refetch()} />
      )}
    </>
  );
}
