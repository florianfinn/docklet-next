// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation. M34 #549: eligibility badge with Refresh in one row; no duplicate Routes heading.

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import { ProxyError, type ProxyApi } from "@/data/http/proxy-api";
import { Button } from "@/shared/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { EligibilityView, ErrorView } from "./controls";
import { Operations } from "./Operations";
import { RouteEditor } from "./RouteEditor";
import { ChangeDialog, type ChangeAction } from "./ChangeDialog";
import { RouteList } from "./RouteList";
import { ManualImportHint } from "./ManualImportHint";
import { Settings } from "./Settings";
import { canInsertImport, drifted, operationRequest, relativeTarget, routeWritesAllowed } from "./model";
import type { Acknowledgements, Change, Instance, Inventory, Operation, ReadResult, Route } from "./types";

type Snapshot = { read: ReadResult; inventory: Inventory };
export function InstanceWorkspace({
  instance,
  api,
  updated,
  removed,
}: {
  instance: Instance;
  api: ProxyApi;
  updated: (i: Instance) => void;
  removed: () => void;
}) {
  const t = useTranslations("proxyContract.ui"),
    [snapshot, setSnapshot] = useState<Snapshot>(),
    [rows, setRows] = useState<Operation[]>([]),
    [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false),
    [uncertain, setUncertain] = useState(false),
    lock = useRef(false),
    active = useRef(false);
  const [editor, setEditor] = useState<{ route?: Route; snapshot: Snapshot }>(),
    [action, setAction] = useState<{ value: ChangeAction; snapshot: Snapshot }>(),
    [refusal, setRefusal] = useState<unknown>();
  const refresh = useCallback(async () => {
    const [configuration, operations] = await Promise.allSettled([
      Promise.all([api.read(instance), api.inventory(instance)]),
      api
        .operations(instance.id)
        .then(rows => Promise.all(rows.map(row => (row.state === "recovery-required" ? api.operation(row.id) : row)))),
    ]);
    if (!active.current) return false;
    if (operations.status === "fulfilled") setRows(operations.value);
    if (configuration.status === "fulfilled") {
      const [read, inventory] = configuration.value;
      setSnapshot({ read, inventory });
    } else setSnapshot(undefined);
    const failure =
      configuration.status === "rejected"
        ? configuration.reason
        : operations.status === "rejected"
          ? operations.reason
          : undefined;
    setError(failure);
    return !failure;
  }, [api, instance]);
  useEffect(() => {
    active.current = true;
    let live = true;
    void Promise.resolve().then(() => {
      if (live) return refresh();
    });
    return () => {
      live = false;
      active.current = false;
    };
  }, [refresh]);
  useEffect(() => {
    if (!rows.some(r => r.state === "pending")) return;
    let stopped = false;
    const timer = setTimeout(() => {
      void Promise.all(rows.filter(r => r.state === "pending").map(r => api.operation(r.id))).then(
        async updates => {
          if (stopped) return;
          setRows(old => old.map(row => updates.find(u => u.id === row.id) ?? row));
          setError(undefined);
          if (updates.some(u => u.state !== "pending")) await refresh();
        },
        error => {
          if (!stopped) {
            setError(error);
            setRows(old => [...old]);
          }
        },
      );
    }, 1500);
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [api, rows, refresh]);
  const run = async (work: () => Promise<void>, isOperation = false): Promise<boolean> => {
    if (lock.current || uncertain) return false;
    lock.current = true;
    setBusy(true);
    setError(undefined);
    try {
      await work();
      if (!active.current) return false;
      await refresh();
      return true;
    } catch (e) {
      if (active.current) {
        const definite = e instanceof ProxyError && e.status !== undefined && e.status < 500;
        if (isOperation) {
          setUncertain(!definite);
          // A definite refusal keeps the draft open beside its reason; an unknown outcome closes it.
          if (definite) setRefusal(e);
          else {
            setEditor(undefined);
            setAction(undefined);
          }
          await refresh();
        }
        if (!isOperation || !definite) setError(e);
      }
      return false;
    } finally {
      lock.current = false;
      if (active.current) setBusy(false);
    }
  };
  const write = (held: Snapshot, target: string, change: Change, ack: Acknowledgements) => {
    setRefusal(undefined);
    void run(async () => {
      const op = await api.operate(instance.id, operationRequest(held.read, target, change, ack));
      if (active.current) {
        setRows(old => [op, ...old.filter(r => r.id !== op.id)]);
        if (op.state === "rejected") setRefusal(op);
        else {
          setEditor(undefined);
          setAction(undefined);
        }
      }
    }, true);
  };
  const blocked = busy || uncertain || rows.some(r => ["pending", "recovery-required"].includes(r.state));
  // Settings stay reachable without a readable snapshot, so a stale instance can still be removed.
  const settings = (inventory?: Inventory) => (
    <Settings
      key={JSON.stringify([instance.name, instance.rootFile, instance.routeDirectory])}
      instance={instance}
      inventory={inventory}
      busy={blocked}
      save={input => {
        void run(async () => {
          const i = await api.update(instance.id, input);
          if (active.current) updated(i);
        });
      }}
      remove={() => {
        void run(async () => {
          await api.remove(instance.id);
          if (active.current) removed();
        });
      }}
    />
  );
  const writable = !!snapshot && routeWritesAllowed(snapshot.read, snapshot.inventory) && !blocked;
  return (
    <div className="grid min-w-0 gap-5">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">{snapshot && <EligibilityView value={snapshot.inventory.eligibility} />}</div>
        <Button
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={() => {
            setEditor(undefined);
            setAction(undefined);
            void refresh().then(ok => {
              if (ok && active.current) setUncertain(false);
            });
          }}
        >
          {t("refresh")}
        </Button>
      </div>
      {uncertain && (
        <p role="alert" className="text-sm">
          {t("uncertain")}
        </p>
      )}
      {error !== undefined && <ErrorView error={error} />}
      {!snapshot && error === undefined && <p role="status">{t("loading")}</p>}
      {!snapshot && error !== undefined && settings(undefined)}
      {snapshot && (
        <>
          {JSON.stringify(snapshot.read.eligibility) !== JSON.stringify(snapshot.inventory.eligibility) && (
            <EligibilityView value={snapshot.read.eligibility} />
          )}
          {snapshot.inventory.runningDiverged && (
            <p role="status" className="text-sm text-amber-700 dark:text-amber-300">
              {t("diverged")}
            </p>
          )}
          {instance.kind === "caddy" && !snapshot.inventory.importPresent && snapshot.inventory.importLine && (
            <div className="grid min-w-0 gap-3 rounded-xl border p-4">
              <pre className="whitespace-pre-wrap break-all font-mono text-xs">{snapshot.inventory.importLine}</pre>
              {canInsertImport(snapshot.inventory) ? (
                <Button
                  className="justify-self-start"
                  disabled={blocked}
                  onClick={() =>
                    setAction({
                      value: {
                        kind: "insert-import",
                        target: relativeTarget(instance, snapshot.inventory.rootFile ?? instance.rootFile ?? ""),
                      },
                      snapshot,
                    })
                  }
                >
                  {t("insertImport")}
                </Button>
              ) : (
                <ManualImportHint instance={instance} />
              )}
            </div>
          )}
          <Tabs defaultValue="routes" className="min-w-0">
            <TabsList variant="line" className="max-w-full justify-start border-b">
              <TabsTrigger value="routes">{t("routes")}</TabsTrigger>
              <TabsTrigger value="settings">{t("settings")}</TabsTrigger>
            </TabsList>
            <TabsContent value="routes" className="grid min-w-0 gap-4 pt-3">
              <RouteList
                entryPoints={snapshot.inventory.entryPoints}
                instance={{ ...instance, identity: snapshot.inventory.identity }}
                routes={snapshot.read.routes ?? []}
                hashes={Object.fromEntries((snapshot.read.files ?? []).map(file => [file.path, file.hash]))}
                writable={writable}
                create={() => setEditor({ snapshot })}
                edit={route => setEditor({ route, snapshot })}
                action={value => setAction({ value, snapshot })}
              />
              <section className="grid min-w-0 gap-2">
                <h3 className="text-sm font-medium">{t("sourceFiles")}</h3>
                {snapshot.read.files?.map(f => (
                  <div
                    key={f.path}
                    className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-lg border p-3"
                  >
                    <span className="break-all font-mono text-xs">{f.path}</span>
                    {drifted(f) && (
                      <Button
                        variant="outline"
                        disabled={blocked}
                        onClick={() => setAction({ value: { kind: "restore-owned", target: f.path }, snapshot })}
                      >
                        {t("restore")}
                      </Button>
                    )}
                  </div>
                ))}
              </section>
              {!!snapshot.read.opaqueFiles?.length && (
                <section className="min-w-0">
                  <h3 className="text-sm font-medium">{t("opaqueFiles")}</h3>
                  <ul className="break-all font-mono text-xs">
                    {snapshot.read.opaqueFiles.map(f => (
                      <li key={f.path}>{f.path}</li>
                    ))}
                  </ul>
                </section>
              )}
            </TabsContent>
            <TabsContent value="settings" className="min-w-0 pt-3">
              {settings(snapshot.inventory)}
            </TabsContent>
          </Tabs>
        </>
      )}
      <Operations
        rows={rows}
        busy={busy || uncertain}
        release={async (id, input) => {
          await run(async () => {
            await api.release(id, input);
          });
        }}
      />
      {editor && (
        <RouteEditor
          instance={instance}
          inventory={editor.snapshot.inventory}
          read={editor.snapshot.read}
          route={editor.route}
          busy={busy}
          refusal={refusal}
          close={() => {
            setEditor(undefined);
            setRefusal(undefined);
          }}
          submit={(target, change, ack) => write(editor.snapshot, target, change, ack)}
        />
      )}
      {action && (
        <ChangeDialog
          action={action.value}
          inventory={action.snapshot.inventory}
          busy={busy}
          caddy={instance.kind === "caddy"}
          refusal={refusal}
          close={() => {
            setAction(undefined);
            setRefusal(undefined);
          }}
          submit={(target, change, ack) => write(action.snapshot, target, change, ack)}
        />
      )}
    </div>
  );
}
