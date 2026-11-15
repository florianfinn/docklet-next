// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original real hub refusal response and HTTP adapter consumer proof.

export const replacementRefusal = {
  "success": true,
  "data": {
    "activity": {
      "operation": "stop-container",
      "id": "1dd08eac-6846-4ba1-bbea-a3e5112cc860",
      "environmentId": "direct",
      "sourceEnvironmentId": "direct",
      "type": "container_stop",
      "status": "failed",
      "resourceType": "container",
      "resourceId": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "resourceName": "app",
      "latestMessage": "run.busy",
      "messageCode": "run.busy",
      "startedBy": {
        "userId": "example-owner",
        "username": "operator",
        "displayName": "Operator"
      },
      "startedAt": "2026-11-13T12:00:00Z",
      "endedAt": "2026-11-13T12:00:00Z",
      "error": "run.busy",
      "createdAt": "2026-11-13T12:00:00Z",
      "updatedAt": "2026-11-13T12:00:00Z",
      "outcomeState": "known",
      "cancelRequested": false
    },
    "messages": [
      {
        "id": "00000000-0000-4000-8000-000000000001",
        "activityId": "1dd08eac-6846-4ba1-bbea-a3e5112cc860",
        "level": "info",
        "message": "run.started",
        "code": "run.started",
        "payload": {
          "actorId": "example-owner",
          "key": "00000000-0000-4000-8000-000000000010",
          "operation": "stop-container"
        },
        "createdAt": "2026-11-13T12:00:00Z"
      },
      {
        "id": "00000000-0000-4000-8000-000000000002",
        "activityId": "1dd08eac-6846-4ba1-bbea-a3e5112cc860",
        "level": "warning",
        "message": "run.busy",
        "code": "run.busy",
        "payload": {
          "actorId": "example-owner",
          "disposition": "refused",
          "releaseAcknowledged": false,
          "effectVerified": false
        },
        "createdAt": "2026-11-13T12:00:00Z"
      }
    ],
    "stepTrace": [],
    "output": [],
    "outputTruncated": false,
    "retainedStartSeq": 1,
    "recovery": {
      "executorState": "pending",
      "disposition": "refused",
      "dispositionBy": "example-owner",
      "dispositionReason": "run.busy",
      "dispositionAt": "2026-11-13T12:00:00Z",
      "releaseAcknowledged": false
    }
  }
} as const;
