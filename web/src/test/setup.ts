import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Concurrent UI suites need scheduling headroom; missing elements still fail within this bound.
configure({ asyncUtilTimeout: 15_000 });

afterEach(() => cleanup());
