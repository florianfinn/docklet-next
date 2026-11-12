import { cleanup, configure } from "@testing-library/react";
import { afterEach, beforeEach } from "vitest";

// Concurrent UI suites need scheduling headroom; missing elements still fail within this bound.
configure({ asyncUtilTimeout: 15_000 });

afterEach(() => cleanup());
// The hub sets its session presence hint with every session cookie; tests model a browser that may hold a session.
// Anonymous-start tests remove it themselves.
beforeEach(() => { if (typeof document !== "undefined") document.cookie = "docklet_session=1; Path=/"; });
