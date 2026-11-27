// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original metadata loss consumer boundary and gesture proofs.


import { describe, expect, it } from "vitest";
import { decodeProxyOperation } from "./proxy-decoder";
import { operation } from "@/features/proxy/proxy.test-support";

const entry = { path: "routes/app.yml", attributes: ["system.posix_acl_access", "user.example"], links: 1, token: Array(65).join("c") };
const decode = (metadataLoss: unknown) => decodeProxyOperation({ success: true, data: operation({ state: "rejected", reason: "metadata-loss", ...{ metadataLoss } as object }) });
describe("metadata loss decoder", () => {
  it("keeps complete entries and optional absent fields byte for byte", () => {
    expect(decode([entry, { path: "routes/other.yml", token: entry.token }]).metadataLoss).toEqual([entry, { path: "routes/other.yml", token: entry.token }]);
    expect(decode([]).metadataLoss).toEqual([]);
  });
  const malformed = [
    null, {}, "loss", Array(17).fill(entry), [null], [[]],
    ...[undefined, 3, "x".repeat(1025)].map(path => [{ ...entry, path }]),
    ...[null, {}, "user.example", Array(65).fill("user.example"), [3], [""], ["x".repeat(256)], ["é".repeat(128)], ["user.\nexample"], ["user.\x7fexample"]].map(attributes => [{ ...entry, attributes }]),
    ...[null, "1", -1, 1025, 1.5].map(links => [{ ...entry, links }]),
    ...[undefined, null, 4, "C".repeat(64), "c".repeat(63), "g".repeat(64)].map(token => [{ ...entry, token }]),
  ];
  it.each(malformed.map((value, index) => ({ value, index })))("rejects malformed loss $index", ({ value }) => {
    expect(() => decode(value)).toThrow("proxy.invalid-response");
  });
});
