// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Trusted command normalization shared by active HTTP admission and executor intake.

package activity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

// Aliases keep the trusted internal port names while sharing active runtime wire vocabulary.
type RuntimeOperation = activitytypes.RuntimeOperation

const (
	RuntimeUpdateProject     = activitytypes.RuntimeUpdateProject
	RuntimeRestoreProject    = activitytypes.RuntimeRestoreProject
	RuntimeDeployProject     = activitytypes.RuntimeDeployProject
	RuntimeDownProject       = activitytypes.RuntimeDownProject
	RuntimeStopProject       = activitytypes.RuntimeStopProject
	RuntimeRecreateContainer = activitytypes.RuntimeRecreateContainer
	RuntimeRedeployContainer = activitytypes.RuntimeRedeployContainer
	RuntimeRestartProject    = activitytypes.RuntimeRestartProject
	RuntimeRedeployProject   = activitytypes.RuntimeRedeployProject
	RuntimeStartContainer    = activitytypes.RuntimeStartContainer
	RuntimeStopContainer     = activitytypes.RuntimeStopContainer
	RuntimeRestartContainer  = activitytypes.RuntimeRestartContainer
)

type RuntimeTarget = activitytypes.RuntimeTarget
type RuntimeOptions = activitytypes.RuntimeOptions

type RuntimeStoreBinding struct {
	StoreGeneration, ManagerID, RegistrationID string
	EnvironmentID, ExecutorGeneration          string
}

// RuntimeCommand is trusted intake after operation and activity-read grants.
// There is deliberately no caller-supplied digest or authorization boolean.
// ProposedRunID is optional; a retained key always keeps its original run ID.
type RuntimeCommand struct {
	Store              RuntimeStoreBinding
	Key, ProposedRunID string
	Actor              usertypes.Actor
	Operation          RuntimeOperation
	Target             RuntimeTarget
	Options            RuntimeOptions
}

type runtimeCanonicalCommandInternal struct {
	command                                   RuntimeCommand
	digest, optionsDigest, gateScope, gateKey string
}

func runtimeTextInternal(value string, max int) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= max && strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func runtimeTargetValidInternal(target RuntimeTarget) bool {
	return runtimeTextInternal(target.ID, 256) && !strings.Contains(target.ID, "~") &&
		(target.Name == nil || runtimeTextInternal(*target.Name, 256)) &&
		(target.ParentProjectID == nil || (target.Kind == "container" && runtimeTextInternal(*target.ParentProjectID, 256) && !strings.Contains(*target.ParentProjectID, "~")))
}

// Hub intake alone accepts a client key. A missing key is never synthesized here.
func runtimeCanonicalizeInternal(command RuntimeCommand) (runtimeCanonicalCommandInternal, error) {
	key, err := runtimeUUIDInternal(command.Key)
	if err != nil {
		return runtimeCanonicalCommandInternal{}, errRuntimeInvalidInternal
	}
	command.Key = key
	return runtimeCanonicalizeSemanticInternal(command)
}

// Pure trusted executor preparation, not receiver acceptance or permission. C2
// supplies authenticated store ownership and attribution before calling this.
// Run-ID deduplication belongs to its receipt transaction, never a hub key lookup.
func runtimeCanonicalizeExecutorInternal(command RuntimeCommand, claimedDigest string) (runtimeCanonicalCommandInternal, error) {
	if command.Key != "" || command.ProposedRunID == "" || command.Store.EnvironmentID != "0" ||
		command.Store.StoreGeneration != command.Store.ExecutorGeneration {
		return runtimeCanonicalCommandInternal{}, errRuntimeInvalidInternal
	}
	// Only attribution crosses the execution boundary; personal preferences do not.
	command.Actor = usertypes.Actor{ID: command.Actor.ID, Username: command.Actor.Username, DisplayName: command.Actor.DisplayName}
	canonical, err := runtimeCanonicalizeSemanticInternal(command)
	if err != nil {
		return runtimeCanonicalCommandInternal{}, err
	}
	if claimedDigest != canonical.digest {
		return runtimeCanonicalCommandInternal{}, &runtimeConflictInternal{}
	}
	return canonical, nil
}

// Both intakes use exactly this validator, owned copy and fixed-array encoder.
// Store/actor validation is structural; it cannot authenticate a wire identity.
func runtimeCanonicalizeSemanticInternal(command RuntimeCommand) (runtimeCanonicalCommandInternal, error) {
	invalid := func() (runtimeCanonicalCommandInternal, error) {
		return runtimeCanonicalCommandInternal{}, errRuntimeInvalidInternal
	}
	if _, ok := runtimeActivityTypeInternal(string(command.Operation), command.Target.Kind); !ok || !runtimeTargetValidInternal(command.Target) {
		return invalid()
	}
	if command.ProposedRunID != "" {
		id, err := runtimeUUIDInternal(command.ProposedRunID)
		if err != nil {
			return invalid()
		}
		command.ProposedRunID = id
	}
	for _, value := range []string{command.Store.StoreGeneration, command.Store.ManagerID, command.Store.RegistrationID,
		command.Store.EnvironmentID, command.Store.ExecutorGeneration, command.Actor.ID} {
		if !runtimeTextInternal(value, 256) {
			return invalid()
		}
	}
	if !utf8.ValidString(command.Actor.Username) || len(command.Actor.Username) > 256 || strings.ContainsFunc(command.Actor.Username, unicode.IsControl) ||
		(command.Actor.DisplayName != nil && (!utf8.ValidString(*command.Actor.DisplayName) || len(*command.Actor.DisplayName) > 256 || strings.ContainsFunc(*command.Actor.DisplayName, unicode.IsControl))) {
		return invalid()
	}
	// Copy all semantic input so caller mutation during resolution cannot alter
	// either the digest or the command subsequently handed to an owner.
	command.Target.ParentProjectID = copyPtrInternal(command.Target.ParentProjectID)
	command.Target.Name = copyPtrInternal(command.Target.Name)
	command.Actor.DisplayName = copyPtrInternal(command.Actor.DisplayName)
	if command.Options.Recreate != nil && command.Operation != RuntimeRecreateContainer {
		return invalid()
	}
	if command.Operation != RuntimeUpdateProject && command.Operation != RuntimeRestoreProject && command.Options.Lifecycle != nil {
		return invalid()
	}
	switch command.Operation {
	case RuntimeRecreateContainer:
		if command.Options.Recreate == nil || command.Options.Deploy != nil || command.Options.RestartServices != nil {
			return invalid()
		}
		owned, err := runtimeOwnedEdit(command.Options.Recreate)
		if err != nil {
			return invalid()
		}
		command.Options.Recreate = owned

	case RuntimeUpdateProject, RuntimeRestoreProject:
		if command.Options.Deploy != nil || command.Options.RestartServices != nil || command.Options.Lifecycle == nil {
			return invalid()
		}
		payload, e := json.Marshal(command.Options.Lifecycle)
		if e != nil || len(payload) > updaterDTO.MaxLifecycleIntentBytes {
			return invalid()
		}
		if json.Unmarshal(payload, new(any)) != nil {
			return invalid()
		}
		var owned = *command.Options.Lifecycle
		if owned.Policy != nil {
			binding := *owned.Policy
			if binding.Revision == 0 || !runtimeTextInternal(binding.PolicyID, 64) || !runtimeTextInternal(binding.JobRunID, 256) {
				return invalid()
			}
			if _, e := runtimeUUIDInternal(binding.ChildID); e != nil {
				return invalid()
			}
			owned.Policy = &binding
		}
		owned.Selections = slices.Clone(owned.Selections)
		for _, hash := range []string{owned.TargetIdentity, owned.SavedRevision, owned.ObservedRevision} {
			b, e := hex.DecodeString(hash)
			if e != nil || len(b) != 32 {
				return invalid()
			}
		}
		if owned.Request.ExpectedRevision != owned.SavedRevision || (command.Operation == RuntimeRestoreProject) != (owned.Request.PointID != "") {
			return invalid()
		}
		revision := owned.BaselineRevision
		if command.Operation == RuntimeRestoreProject {
			revision = owned.PointRevision
		}
		if b, e := hex.DecodeString(revision); e != nil || len(b) != 32 {
			return invalid()
		}
		if len(owned.Selections) > updaterDTO.MaxLifecycleServices {
			return invalid()
		}
		names := map[string]bool{}
		for _, sel := range owned.Selections {
			image, e := hex.DecodeString(strings.TrimPrefix(sel.ImageID, "sha256:"))
			platform := strings.Split(sel.Platform, "/")
			if !runtimeTextInternal(sel.Service, 256) || names[sel.Service] || !runtimeTextInternal(sel.Reference, 1024) || !strings.HasPrefix(sel.ImageID, "sha256:") || e != nil || len(image) != 32 || len(platform) < 2 || len(platform) > 3 || platform[0] == "" || platform[1] == "" {
				return invalid()
			}
			if command.Operation == RuntimeRestoreProject {
				if sel.Kind != "restore" {
					return invalid()
				}
			} else {
				if sel.Kind != "version" && sel.Kind != "rebuild" {
					return invalid()
				}
				digest, e := hex.DecodeString(strings.TrimPrefix(sel.Digest, "sha256:"))
				if !strings.HasPrefix(sel.Digest, "sha256:") || e != nil || len(digest) != 32 {
					return invalid()
				}
			}
			names[sel.Service] = true
		}
		command.Options = RuntimeOptions{Lifecycle: &owned}
	case RuntimeDeployProject, RuntimeRedeployProject:
		if command.Options.RestartServices != nil {
			return invalid()
		}
		options := projecttypes.DeployOptions{}
		if command.Options.Deploy != nil {
			options = *command.Options.Deploy
		}
		options.PullPolicy = strings.ToLower(strings.TrimSpace(options.PullPolicy))
		if options.PullPolicy == "if_not_present" {
			options.PullPolicy = "missing"
		}
		switch options.PullPolicy {
		case "", "missing", "always", "never":
		default:
			return invalid()
		}
		command.Options = RuntimeOptions{Deploy: &options}
	case RuntimeRestartProject:
		if command.Options.Deploy != nil {
			return invalid()
		}
		services := slices.Clone(command.Options.RestartServices)
		if len(services) > 256 {
			return invalid()
		}
		for _, name := range services {
			if !runtimeTextInternal(name, 256) {
				return invalid()
			}
		}
		slices.Sort(services)
		command.Options.RestartServices = slices.Compact(services)
	default:
		if command.Options.Deploy != nil || command.Options.RestartServices != nil {
			return invalid()
		}
	}
	// Arrays give a fixed-field, length-delimited encoding, independent of object
	// order. The nullable parent is explicit; display and transport are excluded.
	// Text validation rejects invalid UTF-8 rather than allowing JSON replacement
	// characters to collapse distinct byte strings into the same semantic intent.
	var options any = [0]string{}
	if command.Options.Recreate != nil {
		options = runtimeRecreateSemantic(*command.Options.Recreate)
	} else if command.Options.Lifecycle != nil {
		options = command.Options.Lifecycle
	} else if deploy := command.Options.Deploy; deploy != nil {
		options = [4]any{deploy.PullPolicy, deploy.ForceRecreate, deploy.RemoveOrphans, deploy.RecreateVolumes}
	} else if command.Operation == RuntimeRestartProject {
		services := command.Options.RestartServices
		if services == nil {
			services = []string{}
		}
		options = services
	}
	optionsDigest := runtimeDigestInternal(options)
	digest := runtimeDigestInternal([5]any{string(command.Operation), command.Target.Kind, command.Target.ID, options, command.Target.ParentProjectID})
	return runtimeCanonicalCommandInternal{command: command, digest: digest, optionsDigest: optionsDigest,
		gateScope: command.Target.Kind + ":" + command.Target.ID, gateKey: string(command.Operation) + ":" + optionsDigest}, nil
}

func runtimeDigestInternal(fields any) string {
	// Only fixed arrays, strings, bools, string slices and nullable strings enter
	// this private encoder; these values cannot fail JSON encoding.
	encoded, _ := json.Marshal(fields)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (c runtimeCanonicalCommandInternal) identity(runID string) runtimeIdentityInternal {
	b := c.command.Store
	return runtimeIdentityInternal{RunID: runID, StoreGeneration: b.StoreGeneration, ManagerID: b.ManagerID,
		RegistrationID: b.RegistrationID, EnvironmentID: b.EnvironmentID, ExecutorGeneration: b.ExecutorGeneration, RequestDigest: c.digest}
}

func (c runtimeCanonicalCommandInternal) binding() runtimeBindingInternal {
	return runtimeBindingInternal{Key: c.command.Key, CallerID: c.command.Actor.ID, CallerName: new(c.command.Actor.Username)}
}
