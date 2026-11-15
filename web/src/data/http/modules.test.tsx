// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original M7 real-producer refusal decoding and rendered bilingual reasons.

import { readFileSync } from "node:fs";
import { useEffect, useState } from "react";
import { screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { renderWithApp } from "@/test/render";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { useCommandError } from "@/data/use-command-error";
import { createModulesApi } from "./modules";

const fixtures=JSON.parse(readFileSync("../types/modules/testdata/problems.json","utf8")) as {transport:string;code:string;status:number;body:unknown}[];
const messages={
 en:{"module-busy":"An operation is still unfinished: apply-1. Wait for it to finish.","module-on":"Switch the module off before deleting its data.","operation-conflict":"Another module operation conflicts with this request.","feature-disabled":"This module is switched off.","module-not-found":"This module page is unavailable."},
 de:{"module-busy":"Ein Vorgang ist noch nicht abgeschlossen: apply-1. Warte auf seinen Abschluss.","module-on":"Schalte das Modul vor dem Löschen seiner Daten aus.","operation-conflict":"Ein anderer Modulvorgang steht dieser Anfrage entgegen.","feature-disabled":"Dieses Modul ist ausgeschaltet.","module-not-found":"Diese Modulseite ist nicht verfügbar."},
};
afterEach(()=>{setAppLocale("de");localStorage.clear();});
for(const fixture of fixtures)for(const locale of ["en","de"] as const)it(`M7 renders ${fixture.code} from real ${fixture.transport} producer in ${locale}`,async()=>{
 setAppLocale(locale==="en"?"en-US":"de");
 const api=createModulesApi("/api",{fetch:async()=>new Response(JSON.stringify(fixture.body),{status:fixture.status,headers:{"Content-Type":"application/problem+json"}}),token:async()=>"example"});
 function Consumer(){const format=useCommandError();const [error,setError]=useState<unknown>();useEffect(()=>{void api.deleteModuleData!("proxy").catch(setError);},[]);return <p role="alert">{error?format(error):"loading"}</p>;}
 renderWithApp(<Consumer/>,createDemoAdapter());
 await waitFor(()=>expect(screen.getByRole("alert").textContent).not.toBe("loading"));
 expect(screen.getByRole("alert").textContent).toBe(messages[locale][fixture.code as keyof typeof messages.en]);
});
