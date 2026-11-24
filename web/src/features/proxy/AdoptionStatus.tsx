// Source: docklet-next/issues/592
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original bilingual adoption outcome and refusal rendering.

import {useTranslations} from "use-intl";
import {PROXY_ADOPTION_OUTCOME_KEYS,PROXY_ADOPTION_REASON_KEYS} from "@/data/wire-catalogue-proxy";
import type {Adoption} from "./types";
export function AdoptionStatus({value}: {value: Adoption}) {
 const t=useTranslations();
 return <div role="status"><p>{t(PROXY_ADOPTION_OUTCOME_KEYS[value.outcome])}</p>{value.reason && <p>{t(PROXY_ADOPTION_REASON_KEYS[value.reason])}</p>}</div>;
}
