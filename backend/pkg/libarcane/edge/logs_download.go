// Source: docklet-next/issues/379
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original route-trie classification of finite container log downloads.

package edge

// Resolve the compiled route instead of interpreting user-supplied container names.
func containerLogsDownloadRequest(method, path string) bool {
	command, ok := ResolveEdgeCommandName(method, path, false).Get()
	return ok && command == "container.logs.download"
}
