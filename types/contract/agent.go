// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38; network contract v6 boundary for #203; M16 bind mounts and backup completion; M17 runtime history and personal preferences; M19 backend contracts and target identity integration; unreleased v17 Compose self-update recovery contract. M21 #437: advisory detail. M23 #435: active image summaries. M24 recovery evidence, validation and reason contracts. M26 volume collection routes and target identity code; optional-module foundation (#472). M36 #568 no-baseline deploy point reason (v21). M37 #586 tailnet setup test and candidate run read (v22).
package contract

const AgentHubVersion = 22
const MinimumAgentVersion = 18
const AgentHubFingerprint = "c2b15889bef9d47a22c686d268c49119e581536584c9fd0b20bff61d304e1ee4"
