// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Scoped update capability and restore-point projection implementation for #229.

package activity

import (
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	"strings"
)

type updateCapabilityRule struct {
	host, admin bool
	grants      []string
}

var updateReadGrants = []string{authz.PermProjectsRead, authz.PermImageUpdatesRead}
var updateWriteGrants = []string{authz.PermProjectsDeploy, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermImageUpdatesCheck, authz.PermActivitiesRead}
var updateNotifyGrants = append(append([]string{}, updateReadGrants...), authz.PermImageUpdatesCheck)
var updateApplyGrants = append(append(append([]string{}, updateNotifyGrants...), updateWriteGrants...), authz.PermJobsManage)

var updateCapabilityRules = map[string]updateCapabilityRule{
	"checkUpdates":        {grants: []string{authz.PermImageUpdatesCheck}},
	"installStackUpdates": {grants: updateWriteGrants}, "restoreRestorePoint": {grants: updateWriteGrants},
	"getUpdateBaseline": {grants: updateReadGrants}, "listRestorePoints": {grants: updateReadGrants},
	"deleteRestorePoint":     {grants: updateWriteGrants},
	"listHostRestorePoints":  {host: true, admin: true, grants: []string{authz.PermImageUpdatesRead}},
	"deleteHostRestorePoint": {host: true, admin: true, grants: []string{authz.PermImageUpdatesRead}},
	"getAutoUpdatePolicy":    {grants: updateReadGrants}, "setAutoUpdate": {grants: updateNotifyGrants},
	"setAutoUpdateApply":   {grants: updateApplyGrants},
	"listSchedule":         {host: true, grants: []string{authz.PermJobsManage}},
	"saveUpdateSchedule":   {host: true, grants: []string{authz.PermJobsManage}},
	"listSystemComponents": {host: true, grants: []string{authz.PermImageUpdatesRead}},
}

func updateCapabilityAllowed(operation string, ref capability.ResourceRef, ps *authz.PermissionSet) bool {
	rule, ok := updateCapabilityRules[operation]
	if !ok || ps == nil {
		return false
	}
	env, raw, scoped := strings.Cut(ref.ID, "~")
	if rule.host {
		if ref.Kind != "host" || scoped || env == "" {
			return false
		}
	} else if ref.Kind != "stack" || !scoped || env == "" || raw == "" || strings.Contains(raw, "~") {
		return false
	}
	if rule.admin && !ps.IsGlobalAdmin() {
		return false
	}
	for _, grant := range rule.grants {
		if !ps.Allows(grant, env) {
			return false
		}
	}
	return true
}
