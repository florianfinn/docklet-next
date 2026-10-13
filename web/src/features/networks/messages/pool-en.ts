// American English address-pool errors of the connect dialog, merged into the networks namespace.
import type { dePoolErrors } from "./pool-de";

export const enPoolErrors: typeof dePoolErrors = {
  ipErrors: { "pool-exhausted": "No free IPv4 address left in the address pool" },
  ip6Errors: { "pool-exhausted": "No free IPv6 address left in the address pool" },
};
