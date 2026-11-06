// Source: docklet-next/issues/308
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original readable outbox notice templates.

package notification

import (
	"fmt"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

// reasonPhrases turns typed outbox reasons into short English phrases; codes stay in the webhook variables.
var reasonPhrases = map[string]string{
	"runtime.failed":                          "the deployment failed",
	"runtime.interrupted":                     "the run was interrupted",
	"backup.failed":                           "the backup failed",
	"volume.backup.interrupted":               "the run was interrupted",
	"volume.restore.interrupted":              "the run was interrupted",
	"volume.restore.interrupted-before-write": "the run was interrupted before any data was written",
	"volume.backup.restart-failed":            "containers could not be restarted afterwards",
	"volume.restore.restart-failed":           "containers could not be restarted afterwards",
	"volume.backup.repository-unavailable":    "the backup repository is unavailable",
	"volume.backup.repository-check-failed":   "the backup repository check failed",
	"volume.backup.engine-unavailable":        "the backup engine is unavailable",
	"volume.backup.engine-timeout":            "the backup engine timed out",
	"volume.backup.engine-output-invalid":     "the backup engine returned an invalid result",
	"volume.backup.snapshot-not-found":        "the snapshot was not found",
	"volume.backup.writer-unconfirmed":        "writing containers could not be paused safely",
	"volume.restore.failed-partial":           "the restore completed only partially",
	"volume.restore.safety-failed":            "the safety backup before the restore failed",
	"volume.helper-unavailable":               "the helper container is unavailable",
	"backup.target.unreachable":               "the backup target is unreachable",
	"backup.target.not-mounted":               "the backup target is not mounted",
	"backup.target.mount-failed":              "the backup target could not be mounted",
	"backup.target.path-missing":              "the backup target path is missing",
	"backup.target.permission-denied":         "the backup target denied access",
	"backup.copy.insufficient-space":          "there is not enough space on the backup target",
	"backup.copy.mount-failed":                "the backup target could not be mounted for the copy",
	"backup.copy.write-failed":                "the copy to the backup target failed",
}

const unknownReasonPhrase = "an unexpected error occurred"

var operationTitles = map[string]string{"deploy-project": "Deployment", "redeploy-project": "Redeploy", "update-project": "Update", "restore-project": "Restore"}

// outboxLinkPaths point notices to the web page that lists the affected resource.
var outboxLinkPaths = map[string]string{"deploy_failed": "/stacks", "backup_failed": "/backups", "host_offline": "/hosts", "auto_heal": "/containers", "sustained_load": "/dashboard", "container_unhealthy": "/containers"}

func reasonPhrase(code string) string {
	if phrase, ok := reasonPhrases[code]; ok {
		return phrase
	}
	return unknownReasonPhrase
}

// outboxText is the plain message of one outbox event; its first line is the title and email subject.
func outboxText(eventType, host string, payload database.JSON, linkBase string) string {
	str := func(key string) string { v, _ := payload[key].(string); return v }
	num := func(key string) float64 {
		switch v := payload[key].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
		return 0
	}
	lines := []string{}
	switch eventType {
	case "deploy_failed":
		title := operationTitles[str("operation")]
		if title == "" {
			title = "Deployment"
		}
		lines = append(lines, fmt.Sprintf("%s failed: stack %s on %s", title, str("stack"), host), "Reason: "+reasonPhrase(str("reason")))
	case "backup_failed":
		title := "Backup"
		if str("operation") == "restore" {
			title = "Restore"
		}
		lines = append(lines, fmt.Sprintf("%s failed: volume %s on %s", title, str("volume"), host), "Reason: "+reasonPhrase(str("reason")))
	case "host_offline":
		if str("outcome") == "online" {
			lines = append(lines, fmt.Sprintf("Host %s is back online", host))
		} else {
			lines = append(lines, fmt.Sprintf("Host %s is offline", host))
		}
	case "auto_heal":
		container := str("container")
		switch str("outcome") {
		case "restarted":
			lines = append(lines, fmt.Sprintf("Auto Heal restarted %s on %s", container, host))
		case "restart-failed":
			lines = append(lines, fmt.Sprintf("Auto Heal could not restart %s on %s", container, host))
		case "limit-reached":
			lines = append(lines, fmt.Sprintf("Auto Heal restart limit reached for %s on %s", container, host), fmt.Sprintf("%.0f restarts in %.0f min; healing is paused for this container", num("restarts"), num("windowMinutes")))
		default:
			lines = append(lines, fmt.Sprintf("Auto Heal checked %s on %s", container, host))
		}
	case "sustained_load":
		metric := map[string]string{"cpu": "CPU", "memory": "memory"}[str("metric")]
		if metric == "" {
			metric = "resource"
		}
		subject := host
		if container := str("container"); container != "" {
			subject = container + " on " + host
		}
		lines = append(lines, fmt.Sprintf("High %s load on %s: %.0f%% above %.0f%% for %.0f min", metric, subject, num("percent"), num("threshold"), num("minutes")))
	case "container_unhealthy":
		container := str("container")
		if str("reason") == "restart-loop" {
			lines = append(lines, fmt.Sprintf("Container %s on %s keeps restarting", container, host), fmt.Sprintf("%.0f restarts in 10 min", num("restarts")))
		} else {
			lines = append(lines, fmt.Sprintf("Container %s on %s is unhealthy", container, host), fmt.Sprintf("Healthcheck failing for %.0f min", num("minutes")))
		}
	default:
		lines = append(lines, fmt.Sprintf("Docklet notice on %s", host))
	}
	if linkBase != "" {
		lines = append(lines, "Open Docklet: "+linkBase+outboxLinkPaths[eventType])
	}
	return strings.Join(lines, "\n")
}
