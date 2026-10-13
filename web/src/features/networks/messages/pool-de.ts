// German address-pool errors of the connect dialog, merged into the networks namespace.
import type { enPoolErrors } from "./pool-en";

export const dePoolErrors: typeof enPoolErrors = {
  ipErrors: { "pool-exhausted": "Im Adressbereich ist keine IPv4-Adresse mehr frei" },
  ip6Errors: { "pool-exhausted": "Im Adressbereich ist keine IPv6-Adresse mehr frei" },
};
