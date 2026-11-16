// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import type { components, operations } from "@/api/generated/schema";

type Schemas = components["schemas"];
export type Instance = Schemas["ProxyInstance"];
export type Discovery = Schemas["ProxyDiscovery"];
export type Inventory = Schemas["ProxyInventory"];
export type ReadResult = Schemas["ProxyReadResult"];
export type Route = Schemas["ProxyRoute"];
export type ProxyFile = Schemas["ProxyFile"];
export type Eligibility = Schemas["ProxyEligibility"];
export type Change = Schemas["ProxyChange"];
export type Acknowledgements = Schemas["ProxyAcknowledgements"];
export type OperationRequest = Schemas["ProxyOperationRequest"];
export type CreateInstance = Schemas["ProxyCreateInstanceRequest"];
export type Settings = Schemas["ProxySettings"];
export type FileState = Schemas["ProxyFileState"];
export type Release = operations["release-proxy-operation"]["requestBody"]["content"]["application/json"];
export type { ProxyOperation as Operation } from "@/data/http/proxy-decoder";
