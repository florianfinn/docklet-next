// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Trusted internal command normalization; no HTTP or executor activation.

package activity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

type RuntimeOperation string

const (
	RuntimeDeployProject    RuntimeOperation = "deploy-project"
	RuntimeDownProject      RuntimeOperation = "down-project"
	RuntimeRestartProject   RuntimeOperation = "restart-project"
	RuntimeRedeployProject  RuntimeOperation = "redeploy-project"
	RuntimeStartContainer   RuntimeOperation = "start-container"
	RuntimeStopContainer    RuntimeOperation = "stop-container"
	RuntimeRestartContainer RuntimeOperation = "restart-container"
)

// RuntimeTarget must already identify a canonical raw resource. Aliases and
// browser host~ID values require boundary resolution before entering this port.
type RuntimeTarget struct {
	Kind            string
	ID              string
	ParentProjectID *string
	Name            *string
}

// RuntimeOptions preserves the selected legacy route shapes. RestartServices is
// a set (Compose selects a dependency graph); an empty set means the whole stack.
type RuntimeOptions struct {
	Deploy          *projecttypes.DeployOptions
	RestartServices []string
}

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

func runtimeCanonicalizeInternal(command RuntimeCommand) (runtimeCanonicalCommandInternal, error) {
	invalid := func() (runtimeCanonicalCommandInternal, error) {
		return runtimeCanonicalCommandInternal{}, errRuntimeInvalidInternal
	}
	if _, ok := runtimeActivityTypeInternal(string(command.Operation), command.Target.Kind); !ok || !runtimeTargetValidInternal(command.Target) {
		return invalid()
	}
	key, err := runtimeUUIDInternal(command.Key)
	if err != nil {
		return invalid()
	}
	command.Key = key
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
	switch command.Operation {
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
	if deploy := command.Options.Deploy; deploy != nil {
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
