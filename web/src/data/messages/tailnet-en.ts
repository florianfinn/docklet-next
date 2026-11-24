// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original tailnet route wire vocabulary texts.

export const enTailnetContract = {
  "codes": {
    "credential-rejected": "The Tailscale credential was rejected. Save it again or run the setup test again after fixing the Tailscale console.",
    "coordination-unreachable": "The Tailscale coordination server cannot be reached. Docklet keeps retrying; check the internet access of the hub.",
    "not-connected": "The hub is not connected to the tailnet. Turn on Tailscale under Settings → Connections or wait until it is connected.",
    "key-mint-failed": "Tailscale could not create a key for this host. Try again in a moment.",
    "mtls-unsupported": "The Tailscale route needs Edge mTLS to be off (EDGE_MTLS_MODE=disabled).",
    "provider-unavailable": "The Tailscale API cannot check the credential right now. Docklet retries automatically; existing hosts stay connected.",
  },
  "connection": {
    "off": "Off",
    "joining": "Joining",
    "connected": "Connected",
    "control-unreachable": "Coordination unreachable",
    "error": "Not connected",
  },
  "provisioning": {
    "not-configured": "No credential",
    "not-applicable": "Keys pasted per host",
    "checking": "Checking credential",
    "ok": "Credential accepted",
    "rejected": "Credential rejected",
    "unavailable": "Credential check unavailable",
  },
  "peer": {
    "online": "Online",
    "offline": "Offline",
    "unknown": "Not seen yet",
  },
};
