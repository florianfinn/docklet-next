// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Sanitized actual project runtime response fields shared with Go boundary tests.

export default {
  "id": "example-project",
  "name": "alpha",
  "composeProjectName": "alpha",
  "status": "running",
  "serviceCount": 1,
  "runningCount": 1,
  "runtimeServices": [{
    "name": "app",
    "image": "registry.example.com/app:1",
    "status": "running",
    "containerId": "c-fallback",
    "containerName": "manual_p-app-1"
  }]
};
