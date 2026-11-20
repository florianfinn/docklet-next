// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import type { components } from "@/api/generated/schema";
import { createCsrfClient } from "@/data/http/csrf-client";
import { createProxyApi, ProxyError } from "@/data/http/proxy-api";
import { decodeProxyOperation } from "@/data/http/proxy-decoder";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { FieldGroup } from "@/shared/ui/field";
import { Choice, ConfirmCheck, ErrorView, ProxyDialog } from "./controls";
import { Operations } from "./Operations";
import type { Instance, Inventory, Operation } from "./types";

type Request = components["schemas"]["ProxyResolverRequest"];
type Provider = NonNullable<Request["challenge"]["provider"]>;
const variables: Record<Provider, string[]> = {
  cloudflare: ["CF_DNS_API_TOKEN", "CF_API_EMAIL", "CF_API_KEY", "CF_ZONE_API_TOKEN"],
  hetzner: ["HETZNER_API_TOKEN"],
  ionos: ["IONOS_API_KEY"],
  inwx: ["INWX_USERNAME", "INWX_PASSWORD", "INWX_SHARED_SECRET"],
  netcup: ["NETCUP_CUSTOMER_NUMBER", "NETCUP_API_KEY", "NETCUP_API_PASSWORD"],
  desec: ["DESEC_TOKEN"],
  duckdns: ["DUCKDNS_TOKEN"],
  httpreq: ["HTTPREQ_ENDPOINT", "HTTPREQ_MODE", "HTTPREQ_USERNAME", "HTTPREQ_PASSWORD"],
};

export function ResolverDialog({ instance, inventory, close }: {
  instance: Instance;
  inventory: Inventory;
  close: () => void;
}) {
  const t = useTranslations("proxyContract.ui.resolverCreate"), ui = useTranslations("proxyContract.ui");
  const controllerRef = useRef(new AbortController());
  const [api] = useState(() => createProxyApi(() => controllerRef.current.signal));
  const [name, setName] = useState(""), [email, setEmail] = useState("");
  const [storage, setStorage] = useState("/certs/acme.json");
  const [challenge, setChallenge] = useState<"http" | "dns">("http");
  const [entry, setEntry] = useState(inventory.entryPoints?.find(ep => !ep.tls)?.name ?? "");
  const [provider, setProvider] = useState<Provider>("cloudflare");
  const [ca, setCA] = useState<Request["ca"]["kind"]>("letsencrypt"), [url, setURL] = useState("");
  const [delay, setDelay] = useState("0"), [resolvers, setResolvers] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [reused, setReused] = useState<Record<string, boolean>>({});
  const [ack, setAck] = useState(false), [busy, setBusy] = useState(false), [uncertain, setUncertain] = useState(false);
  const [result, setResult] = useState<Operation>(), [error, setError] = useState<unknown>();
  const lock = useRef(false);
  useEffect(() => {
    controllerRef.current = new AbortController();
    return () => controllerRef.current.abort();
  }, []);
  useEffect(() => {
    if (result?.state !== "pending") return;
    let live = true;
    const timer = setTimeout(() => {
      void api.operation(result.id).then(value => {
        if (live) setResult(value);
      }, () => { if (live) setUncertain(true); });
    }, 1500);
    return () => { live = false; clearTimeout(timer); };
  }, [api, result]);
  const terminal = result && !["pending", "recovery-required"].includes(result.state);
  const creating = busy || result?.state === "pending";
  const valid = /^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$/.test(name) && !!email && storage.startsWith("/") &&
    !inventory.resolvers?.includes(name) && (challenge === "dns" || !!entry) && (ca !== "custom" || url.startsWith("https://"));
  const submit = async () => {
    if (lock.current || !ack || !valid || result || uncertain) return;
    const controller=controllerRef.current;
    lock.current = true; setBusy(true); setError(undefined);
    const input: Request = {
      name, email, storage, ca: { kind: ca, ...(ca === "custom" ? { url } : {}) },
      challenge: challenge === "http" ? { kind: "http", entryPoint: entry, delayBeforeCheck: 0 } : {
        kind: "dns", provider, delayBeforeCheck: Number(delay),
        resolvers: resolvers.split(",").map(v => v.trim()).filter(Boolean),
      },
      variables: challenge === "http" ? [] : variables[provider].filter(key => reused[key] || values[key]).map(key =>
        ({ name: key, ...(reused[key] ? {} : { value: values[key] }) })),
    };
    // Keep credentials in this one live dispatch only, never in query caches,
    // operation state, local storage, or retry state.
    setValues({});
    try {
      const response = await createCsrfClient("/api", (...args) => fetch(...args)).fetch(`/api/modules/proxy/instances/${encodeURIComponent(instance.id)}/resolvers`, {
        method: "POST", signal: controller.signal,
        headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify(input),
      });
      const body: unknown = await response.json();
      controller.signal.throwIfAborted();
      if (!response.ok) {
        const code = (body as {code?: unknown}).code;
        throw new ProxyError(typeof code === "string" ? code : "invalid-input", undefined, response.status);
      }
      setResult(decodeProxyOperation(body));
    } catch (failure) {
      if (!controller.signal.aborted) {
        setError(failure);
        setUncertain(!(failure instanceof ProxyError && failure.status !== undefined && failure.status < 500));
      }
    } finally {
      lock.current = false;
      if (!controller.signal.aborted) setBusy(false);
    }
  };
  return <ProxyDialog title={t("create")} description={t("hint")} busy={creating} onClose={() => {
    if (result || uncertain) window.location.reload(); else close();
  }}>
    <p className="text-sm">{t("envDisclosure")}</p>
    <p className="text-sm">{t("injection")}</p>
    {error !== undefined && <ErrorView error={error} />}
    {uncertain && <p role="alert">{ui("uncertain")}</p>}
    {!result && !uncertain && <form onSubmit={event => { event.preventDefault(); void submit(); }}>
      <fieldset disabled={creating}><FieldGroup>
        <TextField id="resolver-name" label={ui("name")} value={name} onChange={setName} />
        <TextField id="resolver-email" label={t("email")} type="email" value={email} onChange={setEmail} />
        <TextField id="resolver-storage" label={t("storage")} value={storage} onChange={setStorage} />
        <Choice id="resolver-ca" label={t("ca")} value={ca} onChange={value => setCA(value as Request["ca"]["kind"])} options={[
          { value: "letsencrypt", label: t("production") }, { value: "letsencrypt-staging", label: t("staging") }, { value: "custom", label: t("custom") },
        ]} />
        {ca === "custom" && <TextField id="resolver-url" label={t("url")} type="url" value={url} onChange={setURL} />}
        <Choice id="resolver-challenge" label={t("challenge")} value={challenge} onChange={value => setChallenge(value as "http" | "dns")} options={[
          { value: "http", label: t("challengeHttp") }, { value: "dns", label: t("challengeDns") },
        ]} />
        {challenge === "http" ? <Choice id="resolver-entry" label={t("entryPoint")} value={entry} onChange={setEntry}
          options={(inventory.entryPoints ?? []).filter(ep => !ep.tls).map(ep => ({ value: ep.name, label: `${ep.name} · ${ep.address}` }))} /> : <>
          <Choice id="resolver-provider" label={t("provider")} value={provider} onChange={value => {
            setProvider(value as Provider); setValues({}); setReused({});
          }} options={(Object.keys(variables) as Provider[]).map(value => ({ value, label: t(`providers.${value}`) }))} />
          {variables[provider].map(key => <FieldGroup key={key}>
            <TextField id={`resolver-${key}`} label={key} type="password" autoComplete="new-password" value={values[key] ?? ""}
              disabled={reused[key]} onChange={value => setValues(old => ({ ...old, [key]: value }))} />
            <ConfirmCheck checked={!!reused[key]} onChange={value => {
              setReused(old => ({ ...old, [key]: value })); setValues(old => ({ ...old, [key]: "" }));
            }}>{t("reuse", { name: key })}</ConfirmCheck>
          </FieldGroup>)}
          <TextField id="resolver-delay" label={t("delay")} type="number" value={delay} onChange={setDelay} />
          <TextField id="resolver-dns" label={t("dns")} value={resolvers} onChange={setResolvers} />
        </>}
        <ConfirmCheck checked={ack} onChange={setAck}>{t("ack")}</ConfirmCheck>
        <Button disabled={!valid || !ack || creating}>{t("create")}</Button>
      </FieldGroup></fieldset>
    </form>}
    {result && <Operations rows={[result]} busy={creating} release={async (id, input) => {
      if (lock.current) return;
      const signal = controllerRef.current.signal;
      lock.current = true; setBusy(true);
      try { const operation = await api.release(id, input); if (!signal.aborted) setResult(operation); }
      catch (failure) { if (!signal.aborted) setError(failure); }
      finally { lock.current = false; if (!signal.aborted) setBusy(false); }
    }} />}
    {(terminal || uncertain) && <Button onClick={() => window.location.reload()}>{ui("refresh")}</Button>}
  </ProxyDialog>;
}
