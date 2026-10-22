// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 4b718d5ec47f383122c59a1dbcbb7f44a15e6261
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Sanitised real hub Direct/Edge sandbox workspace envelopes; no operational metadata.

import type { WorkspaceTree } from "@/data/api";

export const directWs: WorkspaceTree = {
  "files": [],
  "fileTreeRevision": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "fileTreeTruncated": false
};
export const directWsCreate: WorkspaceTree = {
  "files": [
    {
      "modTime": "2026-10-22T18:34:24.458685928+02:00",
      "name": "hello.txt",
      "path": "/hello.txt",
      "relativePath": "hello.txt",
      "mode": "-rw-r--r--",
      "size": 9,
      "isDirectory": false,
      "isSymlink": false,
      "editable": true
    }
  ],
  "fileTreeRevision": "6e4ba89f4e75bd33130500551884426227608098662c32ffcdc067e499df1de4",
  "fileTreeTruncated": false
};
export const edgeWs: WorkspaceTree = {
  "files": [],
  "fileTreeRevision": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
  "fileTreeTruncated": false
};
export const edgeWsCreate: WorkspaceTree = {
  "files": [
    {
      "modTime": "2026-10-22T18:34:25.662663857+02:00",
      "name": "hello.txt",
      "path": "/hello.txt",
      "relativePath": "hello.txt",
      "mode": "-rw-r--r--",
      "size": 9,
      "isDirectory": false,
      "isSymlink": false,
      "editable": true
    }
  ],
  "fileTreeRevision": "1b5e4f434dbda10ce3de99dc58ff1531a57119d00891c745c4eabfe8f2fbe7ff",
  "fileTreeTruncated": false
};

export const staleWorkspace = { title: "Conflict", status: 409, detail: "volume workspace changed; refresh it and try again" };
export const missingWorkspaceFile = { title: "Not Found", status: 404, detail: "volume workspace file not found" };
