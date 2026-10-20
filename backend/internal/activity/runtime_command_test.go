// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Seven typed input shapes, canonical digest and retained-key regressions.

package activity

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeCommandSevenShapes(t *testing.T) {
	f := b1Fixture(t)
	for _, operation := range []RuntimeOperation{RuntimeDeployProject, RuntimeDownProject, RuntimeRestartProject, RuntimeRedeployProject,
		RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer} {
		t.Run(string(operation), func(t *testing.T) {
			command := f.command()
			command.Operation = operation
			if operation == RuntimeStartContainer || operation == RuntimeStopContainer || operation == RuntimeRestartContainer {
				command.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}
			}
			canonical, err := runtimeCanonicalizeInternal(command)
			require.NoError(t, err)
			require.Len(t, canonical.digest, 64)
			require.Len(t, canonical.optionsDigest, 64)
			require.Equal(t, command.Target.Kind+":"+command.Target.ID, canonical.gateScope)
			require.Equal(t, string(operation)+":"+canonical.optionsDigest, canonical.gateKey)
		})
	}
}

func TestRuntimeCommandEquivalentOptionsAndOwnedCopies(t *testing.T) {
	f := b1Fixture(t)
	for _, op := range []RuntimeOperation{RuntimeDeployProject, RuntimeRedeployProject} {
		command := f.command()
		command.Operation = op
		nilOptions, err := runtimeCanonicalizeInternal(command)
		require.NoError(t, err)
		command.Options.Deploy = &projecttypes.DeployOptions{}
		zeroOptions, err := runtimeCanonicalizeInternal(command)
		require.NoError(t, err)
		require.Equal(t, nilOptions.digest, zeroOptions.digest)
		for _, policy := range []string{"missing", " MISSING ", "if_not_present"} {
			command.Options.Deploy.PullPolicy = policy
			got, err := runtimeCanonicalizeInternal(command)
			require.NoError(t, err)
			require.Equal(t, "missing", got.command.Options.Deploy.PullPolicy)
			command.Options.Deploy.PullPolicy = "missing"
			expected, err := runtimeCanonicalizeInternal(command)
			require.NoError(t, err)
			require.Equal(t, expected.digest, got.digest)
		}
		command.Options.Deploy = &projecttypes.DeployOptions{RecreateVolumes: true}
		got, err := runtimeCanonicalizeInternal(command)
		require.NoError(t, err)
		command.Options.Deploy.RecreateVolumes = false
		require.True(t, got.command.Options.Deploy.RecreateVolumes)
	}
	command := f.command()
	command.Operation = RuntimeRestartProject
	command.Options.RestartServices = []string{"worker", "api", "api"}
	canonical, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	require.Equal(t, []string{"api", "worker"}, canonical.command.Options.RestartServices)
	command.Options.RestartServices[0] = "changed"
	require.Equal(t, []string{"api", "worker"}, canonical.command.Options.RestartServices)
	command.Options.RestartServices = []string{"api", "worker"}
	other, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	require.Equal(t, canonical.digest, other.digest)
	command.Options.RestartServices = nil
	empty, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	command.Options.RestartServices = []string{}
	emptySlice, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	require.Equal(t, empty.digest, emptySlice.digest)
}

func TestRuntimeCommandDigestDimensions(t *testing.T) {
	f := b1Fixture(t)
	base := f.command()
	canonical, err := runtimeCanonicalizeInternal(base)
	require.NoError(t, err)
	changes := []struct {
		name   string
		change func(*RuntimeCommand)
	}{
		{"operation", func(c *RuntimeCommand) { c.Operation = RuntimeRedeployProject }},
		{"target", func(c *RuntimeCommand) { c.Target.ID = "example-other-stack" }},
		{"pull-policy", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "always"} }},
		{"force-recreate", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{ForceRecreate: true} }},
		{"remove-orphans", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{RemoveOrphans: true} }},
		{"recreate-volumes", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{RecreateVolumes: true} }},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			c := base
			change.change(&c)
			changed, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			require.NotEqual(t, canonical.digest, changed.digest)
			if change.name == "target" {
				require.Equal(t, canonical.gateKey, changed.gateKey)
			}
		})
	}
	for _, parent := range []*string{nil, new("example-parent-a"), new("example-parent-b")} {
		c := base
		c.Operation = RuntimeStartContainer
		c.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: parent}
		first, err := runtimeCanonicalizeInternal(c)
		require.NoError(t, err)
		c.Target.ParentProjectID = new("example-parent-c")
		second, err := runtimeCanonicalizeInternal(c)
		require.NoError(t, err)
		require.NotEqual(t, first.digest, second.digest)
		require.Equal(t, first.gateKey, second.gateKey)
	}
	display := base
	display.Target.Name = new("Different display")
	display.Actor.ID, display.Actor.Username = "example-other-actor", "Different actor"
	display.Store.EnvironmentID, display.Store.RegistrationID = "example-other-environment", "example-other-registration"
	display.Key, display.ProposedRunID = uuid.NewString(), uuid.NewString()
	changed, err := runtimeCanonicalizeInternal(display)
	require.NoError(t, err)
	require.Equal(t, canonical.digest, changed.digest)
	// JSON fixed fields distinguish delimiters and explicit null from strings.
	require.NotEqual(t, runtimeDigestInternal([2]string{"a:b", "c"}), runtimeDigestInternal([2]string{"a", "b:c"}))
}

func TestRuntimeCommandRetainedKeySemanticMismatch(t *testing.T) {
	for _, change := range []string{"operation", "target", "pull-policy", "force-recreate", "remove-orphans", "recreate-volumes", "restart-services", "parent", "nil-parent"} {
		t.Run(change, func(t *testing.T) {
			f := b1Fixture(t)
			command := f.command()
			if change == "parent" || change == "nil-parent" {
				command.Operation = RuntimeStartContainer
				command.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}
			}
			if change == "restart-services" {
				command.Operation = RuntimeRestartProject
			}
			original, err := f.service.Admit(t.Context(), command)
			require.NoError(t, err)
			before := runtimeBoundarySnapshot(t, f.store)
			switch change {
			case "operation":
				command.Operation = RuntimeDownProject
			case "target":
				command.Target.ID = "example-other-stack"
			case "pull-policy":
				command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			case "force-recreate":
				command.Options.Deploy = &projecttypes.DeployOptions{ForceRecreate: true}
			case "remove-orphans":
				command.Options.Deploy = &projecttypes.DeployOptions{RemoveOrphans: true}
			case "recreate-volumes":
				command.Options.Deploy = &projecttypes.DeployOptions{RecreateVolumes: true}
			case "restart-services":
				command.Options.RestartServices = []string{"api"}
			case "parent":
				command.Target.ParentProjectID = new("example-other-parent")
			case "nil-parent":
				command.Target.ParentProjectID = nil
			}
			command.ProposedRunID = uuid.NewString()
			result, err := f.service.Admit(t.Context(), command)
			b1Mismatch(t, result, err, original.ActivityID)
			require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
			require.EqualValues(t, 1, f.resolver.calls.Load(), "retained mismatch must not fresh-resolve")
		})
	}
}

func TestRuntimeCommandInvalidBeforeWrites(t *testing.T) {
	f := b1Fixture(t)
	before := runtimeBoundarySnapshot(t, f.store)
	for _, mutate := range []func(*RuntimeCommand){
		func(c *RuntimeCommand) { c.Key = "invalid" }, func(c *RuntimeCommand) { c.ProposedRunID = "invalid" },
		func(c *RuntimeCommand) { c.Operation = "exec" }, func(c *RuntimeCommand) { c.Target.Kind = "container" },
		func(c *RuntimeCommand) { c.Target.ID = "0~example-stack" }, func(c *RuntimeCommand) { c.Target.ParentProjectID = new("example-parent") },
		func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "arbitrary"} },
		func(c *RuntimeCommand) { c.Options.RestartServices = []string{"api"} },
		func(c *RuntimeCommand) {
			c.Operation = RuntimeDownProject
			c.Options.Deploy = &projecttypes.DeployOptions{}
		},
		func(c *RuntimeCommand) {
			c.Operation = RuntimeStartContainer
			c.Target.Kind = "container"
			c.Options.Deploy = &projecttypes.DeployOptions{}
		},
		func(c *RuntimeCommand) { c.Operation = RuntimeRestartProject; c.Options.RestartServices = []string{""} },
		func(c *RuntimeCommand) { c.Actor.ID = "" }, func(c *RuntimeCommand) { c.Store.EnvironmentID = "" },
		func(c *RuntimeCommand) { c.Target.ID = "example-\xff" },
		func(c *RuntimeCommand) { c.Target.Name = new("example-\xff") },
		func(c *RuntimeCommand) {
			c.Operation = RuntimeStartContainer
			c.Target.Kind = "container"
			c.Target.ParentProjectID = new("example-\xff")
		},
		func(c *RuntimeCommand) { c.Actor.Username = "example-\xff" },
		func(c *RuntimeCommand) { c.Options.RestartServices = []string{} },
		func(c *RuntimeCommand) { c.Operation = RuntimeDownProject; c.Options.RestartServices = []string{} },
		func(c *RuntimeCommand) {
			c.Operation = RuntimeStopContainer
			c.Target.Kind = "container"
			c.Options.RestartServices = []string{}
		},
	} {
		c := f.command()
		mutate(&c)
		result, err := f.service.Admit(t.Context(), c)
		b1Zero(t, result)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
		require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	}
	require.Zero(t, f.resolver.calls.Load())
	require.Equal(t, activitytypes.RuntimeOutcomeUnknown, runtimeKnowledgeInternal(&RuntimeReceipt{ExecutorState: new("pending")}).OutcomeState)
}

// These fixed JSON arrays and SHA-256 values were checked against accepted F-b
// before extracting its normalizer; neither expected value calls the new helper.
func TestRuntimeCommandAcceptedDigestGoldens(t *testing.T) {
	for _, test := range []struct {
		operation                                      RuntimeOperation
		target                                         RuntimeTarget
		options                                        RuntimeOptions
		encoded, optionsEncoded, digest, optionsDigest string
	}{
		{RuntimeOperation("deploy-project"), RuntimeTarget{Kind: "stack", ID: "example-stack"}, RuntimeOptions{}, `["deploy-project","stack","example-stack",["",false,false,false],null]`, `["",false,false,false]`, "98d48fcec2f31ab6cd111a1788a15b10c8e82a0b85d648c23626d116734a0658", "3f00bb53a246ced1e55625b20759e565c49a57091adc31f7bf39313498274900"},
		{RuntimeOperation("down-project"), RuntimeTarget{Kind: "stack", ID: "example-stack"}, RuntimeOptions{}, `["down-project","stack","example-stack",[],null]`, `[]`, "639622b73e28ddfb13b234bf23b21279bbf860470d182aecd99d8c5363b6c7fb", "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"},
		{RuntimeOperation("restart-project"), RuntimeTarget{Kind: "stack", ID: "example-stack"}, RuntimeOptions{}, `["restart-project","stack","example-stack",[],null]`, `[]`, "9d282f4de33768d05bc7a981f71e410ddfa2b5d2f9e281e91c54f9c2608cdbbc", "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"},
		{RuntimeOperation("redeploy-project"), RuntimeTarget{Kind: "stack", ID: "example-stack"}, RuntimeOptions{}, `["redeploy-project","stack","example-stack",["",false,false,false],null]`, `["",false,false,false]`, "64b804ed24a4781426e9c92c1471672d59ff0dff467bd9d4458b57b67f8bdf1f", "3f00bb53a246ced1e55625b20759e565c49a57091adc31f7bf39313498274900"},
		{RuntimeOperation("start-container"), RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}, RuntimeOptions{}, `["start-container","container","example-container",[],"example-parent"]`, `[]`, "ae8f2dacfa69ad48fdb0c3f0b30c8580b8e937aacee719e6e9530d45d19baafa", "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"},
		{RuntimeOperation("stop-container"), RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}, RuntimeOptions{}, `["stop-container","container","example-container",[],"example-parent"]`, `[]`, "12139b51f20e44faf4a37123604bb2e8536936f8559d79e431a4b8a4f8a92e1b", "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"},
		{RuntimeOperation("restart-container"), RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}, RuntimeOptions{}, `["restart-container","container","example-container",[],"example-parent"]`, `[]`, "e8e9f617661a9bf6d4d737431ea4e83f999444e312977ffc61022928615f85a3", "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"},
		{RuntimeOperation("deploy-project"), RuntimeTarget{Kind: "stack", ID: "example-stack"}, RuntimeOptions{Deploy: &projecttypes.DeployOptions{PullPolicy: " IF_NOT_PRESENT ", ForceRecreate: true, RemoveOrphans: true, RecreateVolumes: true}}, `["deploy-project","stack","example-stack",["missing",true,true,true],null]`, `["missing",true,true,true]`, "3adee0f0ed71b9ba80b9818e82a7f846654adcb25d61a322a14d8461e9719753", "7c1691a8bbb0e5da0c2302daec906f0d1dd0800bf70c3ad9041adab1fcc8f5d0"},
		{RuntimeOperation("restart-project"), RuntimeTarget{Kind: "stack", ID: "example-stack"}, RuntimeOptions{RestartServices: []string{"worker", "api", "api"}}, `["restart-project","stack","example-stack",["api","worker"],null]`, `["api","worker"]`, "daff3af9cc2f18eb248f6f8212f9330f716d40ccdaecda6f6443b43e2177d451", "8183c5a8db045e82629dadf5720ed362ed88dd6df54bcdedd430162bbf205852"},
		{RuntimeOperation("start-container"), RuntimeTarget{Kind: "container", ID: "example-container"}, RuntimeOptions{}, `["start-container","container","example-container",[],null]`, `[]`, "a9cf86be559fc99026bb71c801026526d6ac68e418fe7efc935f0ffd9883a6e6", "4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945"},
	} {
		t.Run(string(test.operation)+test.optionsEncoded+test.encoded, func(t *testing.T) {
			c := c1Command()
			c.Operation, c.Target, c.Options = test.operation, test.target, test.options
			got, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			require.Equal(t, test.digest, got.digest, "accepted semantic digest")
			require.Equal(t, test.optionsDigest, got.optionsDigest, "accepted options digest")
			require.Equal(t, c.Target.Kind+":"+c.Target.ID, got.gateScope)
			require.Equal(t, string(c.Operation)+":"+test.optionsDigest, got.gateKey)
			executor, err := runtimeCanonicalizeExecutorInternal(c1ExecutorCommand(c), test.digest)
			require.NoError(t, err)
			require.Equal(t, test.digest, executor.digest, "accepted executor semantic digest")
			require.Equal(t, test.optionsDigest, executor.optionsDigest)
			require.Equal(t, got.gateScope, executor.gateScope)
			require.Equal(t, got.gateKey, executor.gateKey)
			require.Equal(t, got.command.Options, executor.command.Options)
			require.Equal(t, got.command.Target, executor.command.Target)
			require.Empty(t, executor.command.Key)
			require.Equal(t, got.command.ProposedRunID, executor.command.ProposedRunID)
			var fields [5]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(test.encoded), &fields))
			require.Equal(t, test.optionsEncoded, string(fields[3]))
		})
	}
}

func c1Command() RuntimeCommand {
	return RuntimeCommand{Store: RuntimeStoreBinding{StoreGeneration: "example-hub-generation", ManagerID: "example-manager",
		RegistrationID: "example-registration", EnvironmentID: "example-host", ExecutorGeneration: "example-executor-generation"},
		Key: "00000000-0000-4000-8000-000000000001", ProposedRunID: "00000000-0000-4000-8000-000000000002",
		Actor:     usertypes.Actor{ID: "example-actor", Username: "Operator", DisplayName: new("Operator display")},
		Operation: RuntimeDeployProject, Target: RuntimeTarget{Kind: "stack", ID: "example-stack", Name: new("Example stack")}}
}

func c1ExecutorCommand(command RuntimeCommand) RuntimeCommand {
	command.Key = ""
	command.Store.EnvironmentID = "0"
	command.Store.StoreGeneration = command.Store.ExecutorGeneration
	return command
}

// Compare the two actual intake paths, then inspect the independently prepared
// semantics in callers below. No test-only alternate normalizer is involved.
func c1Both(t *testing.T, command RuntimeCommand) [2]runtimeCanonicalCommandInternal {
	t.Helper()
	hub, err := runtimeCanonicalizeInternal(command)
	require.NoError(t, err)
	executor, err := runtimeCanonicalizeExecutorInternal(c1ExecutorCommand(command), hub.digest)
	require.NoError(t, err)
	require.Equal(t, hub.digest, executor.digest)
	require.Equal(t, hub.optionsDigest, executor.optionsDigest)
	require.Equal(t, hub.gateScope, executor.gateScope)
	require.Equal(t, hub.gateKey, executor.gateKey)
	require.Equal(t, hub.command.Target, executor.command.Target)
	require.Equal(t, hub.command.Options, executor.command.Options)
	require.Empty(t, executor.command.Key)
	return [2]runtimeCanonicalCommandInternal{hub, executor}
}

func TestRuntimeCommandC1IntakeBoundaries(t *testing.T) {
	base := c1Command()
	canonical := c1Both(t, base)
	require.Equal(t, "example-host", canonical[0].command.Store.EnvironmentID)
	require.Equal(t, "example-hub-generation", canonical[0].command.Store.StoreGeneration)
	require.Equal(t, "example-executor-generation", canonical[0].command.Store.ExecutorGeneration)
	require.Equal(t, "0", canonical[1].command.Store.EnvironmentID)
	require.Equal(t, "example-executor-generation", canonical[1].command.Store.StoreGeneration)
	require.Equal(t, canonical[0].command.ProposedRunID, canonical[1].command.ProposedRunID)
	t.Run("hub-missing-key", func(t *testing.T) {
		c := base
		c.Key = ""
		got, err := runtimeCanonicalizeInternal(c)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
		require.Equal(t, runtimeCanonicalCommandInternal{}, got)
	})
	t.Run("hub-optional-run", func(t *testing.T) {
		c := base
		c.ProposedRunID = ""
		got, err := runtimeCanonicalizeInternal(c)
		require.NoError(t, err)
		require.Empty(t, got.command.ProposedRunID)
	})
	for _, test := range []struct {
		name   string
		mutate func(*RuntimeCommand)
	}{
		{"executor-missing-run", func(c *RuntimeCommand) { c.ProposedRunID = "" }},
		{"executor-invalid-run", func(c *RuntimeCommand) { c.ProposedRunID = "invalid" }},
		{"executor-client-key", func(c *RuntimeCommand) { c.Key = base.Key }},
		{"executor-invalid-client-key", func(c *RuntimeCommand) { c.Key = "invalid" }},
		{"executor-hub-environment", func(c *RuntimeCommand) { c.Store.EnvironmentID = "example-host" }},
		{"executor-hub-generation", func(c *RuntimeCommand) { c.Store.StoreGeneration = "example-hub-generation" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := c1ExecutorCommand(base)
			test.mutate(&c)
			got, err := runtimeCanonicalizeExecutorInternal(c, canonical[0].digest)
			require.ErrorIs(t, err, errRuntimeInvalidInternal)
			require.Equal(t, runtimeCanonicalCommandInternal{}, got)
		})
	}
	for _, claim := range []string{"", "invalid", strings.Repeat("0", 64), strings.ToUpper(canonical[0].digest)} {
		got, err := runtimeCanonicalizeExecutorInternal(c1ExecutorCommand(base), claim)
		var conflict *runtimeConflictInternal
		require.ErrorAs(t, err, &conflict)
		require.Equal(t, runtimeCanonicalCommandInternal{}, got)
		require.NotContains(t, err.Error(), base.ProposedRunID)
		require.NotContains(t, err.Error(), canonical[0].digest)
	}
	// Attribution can change, but never creates preferences or authority at executor intake.
	base.Actor.Preferences.ThemeMode = new("dark")
	copies := c1Both(t, base)
	require.Equal(t, usertypes.Preferences{}, copies[1].command.Actor.Preferences)
	require.Equal(t, canonical[0].digest, copies[1].digest)
}

func TestRuntimeCommandC1SemanticAndCopyMatrix(t *testing.T) {
	for _, operation := range []RuntimeOperation{RuntimeDeployProject, RuntimeRedeployProject} {
		t.Run(string(operation), func(t *testing.T) {
			base := c1Command()
			base.Operation = operation
			initial := c1Both(t, base)
			for _, change := range []struct {
				name   string
				mutate func(*RuntimeCommand)
			}{
				{"target", func(c *RuntimeCommand) { c.Target.ID = "example-other" }},
				{"pull-policy", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "always"} }},
				{"force-recreate", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{ForceRecreate: true} }},
				{"remove-orphans", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{RemoveOrphans: true} }},
				{"recreate-volumes", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{RecreateVolumes: true} }},
			} {
				t.Run(change.name, func(t *testing.T) {
					c := base
					change.mutate(&c)
					changed := c1Both(t, c)
					for i := range initial {
						require.NotEqual(t, initial[i].digest, changed[i].digest)
						// An unchanged claimed digest must refuse the changed semantic payload.
						got, err := runtimeCanonicalizeExecutorInternal(c1ExecutorCommand(c), initial[i].digest)
						var conflict *runtimeConflictInternal
						require.ErrorAs(t, err, &conflict)
						require.Equal(t, runtimeCanonicalCommandInternal{}, got)
					}
				})
			}
			base.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: " IF_NOT_PRESENT ", ForceRecreate: true, RemoveOrphans: true, RecreateVolumes: true}
			owned := c1Both(t, base)
			*base.Target.Name = "changed"
			*base.Actor.DisplayName = "changed"
			*base.Options.Deploy = projecttypes.DeployOptions{}
			for _, got := range owned {
				require.Equal(t, "Example stack", *got.command.Target.Name)
				require.Equal(t, "Operator display", *got.command.Actor.DisplayName)
				require.Equal(t, projecttypes.DeployOptions{PullPolicy: "missing", ForceRecreate: true, RemoveOrphans: true, RecreateVolumes: true}, *got.command.Options.Deploy)
			}
		})
	}
	base := c1Command()
	base.Operation = RuntimeRestartProject
	empty := c1Both(t, base)
	base.Options.RestartServices = []string{}
	zero := c1Both(t, base)
	for i := range empty {
		require.Equal(t, empty[i].digest, zero[i].digest)
	}
	base.Options.RestartServices = []string{"worker", "api", "api"}
	owned := c1Both(t, base)
	base.Options.RestartServices[0] = "changed"
	for i, got := range owned {
		require.Equal(t, []string{"api", "worker"}, got.command.Options.RestartServices)
		require.NotEqual(t, empty[i].digest, got.digest)
	}
	base.Options.RestartServices = []string{"api", "worker"}
	reordered := c1Both(t, base)
	for i := range owned {
		require.Equal(t, owned[i].digest, reordered[i].digest)
	}
	base.Options = RuntimeOptions{}
	base.Operation = RuntimeStartContainer
	base.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent-a"), Name: new("Example container")}
	original := c1Both(t, base)
	*base.Target.ParentProjectID = "changed"
	for _, got := range original {
		require.Equal(t, "example-parent-a", *got.command.Target.ParentProjectID)
	}
	var digests []string
	for _, parent := range []*string{nil, new("example-parent-a"), new("example-parent-b")} {
		base.Target.ParentProjectID = parent
		pair := c1Both(t, base)
		require.NotContains(t, digests, pair[0].digest)
		digests = append(digests, pair[0].digest)
	}
	original = c1Both(t, base)
	base.Target.Name = new("Other display")
	base.Actor.ID = "example-other-actor"
	base.Actor.Username = "Other actor"
	base.Actor.DisplayName = new("Other actor display")
	base.Store.ManagerID = "example-other-manager"
	base.Store.RegistrationID = "example-other-registration"
	base.Store.ExecutorGeneration = "example-other-generation"
	base.Store.StoreGeneration = "example-other-hub-generation"
	base.ProposedRunID = "00000000-0000-1000-8000-000000000003"
	base.Key = "00000000-0000-1000-8000-000000000004"
	display := c1Both(t, base)
	for i := range original {
		require.Equal(t, original[i].digest, display[i].digest)
	}
	// Exercise delimiter boundaries through real commands rather than the encoder alone.
	base.Target.ID = "a:b"
	base.Target.ParentProjectID = new("c")
	a := c1Both(t, base)
	base.Target.ID = "a"
	base.Target.ParentProjectID = new("b:c")
	b := c1Both(t, base)
	for i := range a {
		require.NotEqual(t, a[i].digest, b[i].digest)
	}
}

func TestRuntimeCommandC1InvalidMatrix(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*RuntimeCommand)
	}{
		{"operation", func(c *RuntimeCommand) { c.Operation = "exec" }},
		{"kind", func(c *RuntimeCommand) { c.Target.Kind = "volume" }},
		{"host-id", func(c *RuntimeCommand) { c.Target.ID = "example-host~example-stack" }},
		{"empty-target", func(c *RuntimeCommand) { c.Target.ID = "" }},
		{"stack-parent", func(c *RuntimeCommand) { c.Target.ParentProjectID = new("example-parent") }},
		{"bad-policy", func(c *RuntimeCommand) { c.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "arbitrary"} }},
		{"deploy-services", func(c *RuntimeCommand) { c.Options.RestartServices = []string{} }},
		{"restart-deploy", func(c *RuntimeCommand) {
			c.Operation = RuntimeRestartProject
			c.Options.Deploy = &projecttypes.DeployOptions{}
		}},
		{"service-empty", func(c *RuntimeCommand) { c.Operation = RuntimeRestartProject; c.Options.RestartServices = []string{""} }},
		{"service-space", func(c *RuntimeCommand) {
			c.Operation = RuntimeRestartProject
			c.Options.RestartServices = []string{" api"}
		}},
		{"parent-host-id", func(c *RuntimeCommand) {
			c.Operation = RuntimeStartContainer
			c.Target.Kind = "container"
			c.Target.ParentProjectID = new("0~example-parent")
		}},
	}
	// Class-wide validation of every stored identity and attribution field.
	fields := []struct {
		name string
		set  func(*RuntimeCommand, string)
	}{
		{"store-generation", func(c *RuntimeCommand, v string) { c.Store.StoreGeneration = v; c.Store.ExecutorGeneration = v }},
		{"executor-generation", func(c *RuntimeCommand, v string) { c.Store.ExecutorGeneration = v; c.Store.StoreGeneration = v }},
		{"manager", func(c *RuntimeCommand, v string) { c.Store.ManagerID = v }},
		{"registration", func(c *RuntimeCommand, v string) { c.Store.RegistrationID = v }},
		{"actor-id", func(c *RuntimeCommand, v string) { c.Actor.ID = v }},
		{"target", func(c *RuntimeCommand, v string) { c.Target.ID = v }},
		{"name", func(c *RuntimeCommand, v string) { c.Target.Name = new(v) }},
		{"username", func(c *RuntimeCommand, v string) { c.Actor.Username = v }},
		{"display", func(c *RuntimeCommand, v string) { c.Actor.DisplayName = new(v) }},
		{"parent", func(c *RuntimeCommand, v string) {
			c.Operation = RuntimeStartContainer
			c.Target.Kind = "container"
			c.Target.ParentProjectID = new(v)
		}},
		{"service", func(c *RuntimeCommand, v string) {
			c.Operation = RuntimeRestartProject
			c.Options.RestartServices = []string{v}
		}},
	}
	for _, field := range fields {
		for _, invalid := range []string{"example-\xff", "example\nvalue", strings.Repeat("a", 257)} {
			mutations = append(mutations, struct {
				name   string
				mutate func(*RuntimeCommand)
			}{field.name + fmt.Sprintf("-%q", invalid), func(c *RuntimeCommand) { field.set(c, invalid) }})
		}
		if field.name != "username" && field.name != "display" {
			mutations = append(mutations, struct {
				name   string
				mutate func(*RuntimeCommand)
			}{field.name + "-empty", func(c *RuntimeCommand) { field.set(c, "") }})
		}
		for _, size := range []int{255, 256} {
			t.Run(fmt.Sprintf("valid-%s-%d", field.name, size), func(t *testing.T) { c := c1Command(); field.set(&c, strings.Repeat("a", size)); c1Both(t, c) })
		}
	}
	for _, operation := range []RuntimeOperation{RuntimeDeployProject, RuntimeDownProject, RuntimeRestartProject, RuntimeRedeployProject, RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer} {
		t.Run("wrong-kind-"+string(operation), func(t *testing.T) {
			c := c1Command()
			c.Operation = operation
			if strings.HasSuffix(string(operation), "container") {
				c.Target.Kind = "stack"
			} else {
				c.Target.Kind = "container"
			}
			c1InvalidBoth(t, c)
		})
		if operation == RuntimeDownProject || strings.HasSuffix(string(operation), "container") {
			for _, options := range []RuntimeOptions{{Deploy: &projecttypes.DeployOptions{}}, {RestartServices: []string{}}, {RestartServices: []string{"api"}}} {
				c := c1Command()
				c.Operation = operation
				c.Options = options
				if strings.HasSuffix(string(operation), "container") {
					c.Target.Kind = "container"
				}
				c1InvalidBoth(t, c)
			}
		}
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) { c := c1Command(); test.mutate(&c); c1InvalidBoth(t, c) })
	}
	for _, size := range []int{255, 256, 257} {
		t.Run(fmt.Sprintf("service-count-%d", size), func(t *testing.T) {
			c := c1Command()
			c.Operation = RuntimeRestartProject
			c.Options.RestartServices = make([]string, size)
			for i := range size {
				c.Options.RestartServices[i] = fmt.Sprintf("service-%03d", i)
			}
			if size > 256 {
				c1InvalidBoth(t, c)
			} else {
				pair := c1Both(t, c)
				require.Len(t, pair[1].command.Options.RestartServices, size)
			}
		})
	}
}

func c1InvalidBoth(t *testing.T, c RuntimeCommand) {
	t.Helper()
	got, err := runtimeCanonicalizeInternal(c)
	require.ErrorIs(t, err, errRuntimeInvalidInternal)
	require.Equal(t, runtimeCanonicalCommandInternal{}, got)
	got, err = runtimeCanonicalizeExecutorInternal(c1ExecutorCommand(c), strings.Repeat("0", 64))
	require.ErrorIs(t, err, errRuntimeInvalidInternal)
	require.Equal(t, runtimeCanonicalCommandInternal{}, got)
}

func TestRuntimeCommandC1WireRoundtrip(t *testing.T) {
	for _, operation := range []RuntimeOperation{RuntimeDeployProject, RuntimeDownProject, RuntimeRestartProject, RuntimeRedeployProject, RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer} {
		t.Run(string(operation), func(t *testing.T) {
			c := c1Command()
			c.Operation = operation
			if strings.HasSuffix(string(operation), "container") {
				c.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-parent")}
			}
			if operation == RuntimeRestartProject {
				c.Options.RestartServices = []string{"worker", "api", "api"}
			}
			if operation == RuntimeDeployProject || operation == RuntimeRedeployProject {
				c.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: " IF_NOT_PRESENT ", RemoveOrphans: true}
			}
			hub, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			envelope := activitytypes.RuntimeExecutionEnvelope{
				Identity:  activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: c.Store.ManagerID, RegistrationID: c.Store.RegistrationID, ExecutorGeneration: c.Store.ExecutorGeneration}, RunID: hub.command.ProposedRunID, RequestDigest: hub.digest},
				Operation: hub.command.Operation, Target: hub.command.Target, Options: hub.command.Options,
				Attribution: activitytypes.RuntimeAttribution{ID: c.Actor.ID, Username: c.Actor.Username, DisplayName: c.Actor.DisplayName}}
			encoded, err := json.Marshal(envelope)
			require.NoError(t, err)
			var wire activitytypes.RuntimeExecutionEnvelope
			require.NoError(t, json.Unmarshal(encoded, &wire))
			again, err := json.Marshal(wire)
			require.NoError(t, err)
			require.Equal(t, string(encoded), string(again))
			// Test-only composition: C2 must authenticate/bind this tuple before conversion.
			command := c1ExecutorCommand(c)
			command.ProposedRunID = wire.Identity.RunID
			command.Operation = wire.Operation
			command.Target = wire.Target
			command.Options = wire.Options
			command.Actor = usertypes.Actor{ID: wire.Attribution.ID, Username: wire.Attribution.Username, DisplayName: wire.Attribution.DisplayName}
			executor, err := runtimeCanonicalizeExecutorInternal(command, wire.Identity.RequestDigest)
			require.NoError(t, err)
			require.Equal(t, hub.digest, executor.digest)
			require.Equal(t, hub.command.Options, executor.command.Options)
			require.Equal(t, hub.command.ProposedRunID, executor.command.ProposedRunID)
			require.Empty(t, executor.command.Key)
			require.NotContains(t, string(encoded), c.Key)
			require.NotContains(t, string(encoded), c.Store.StoreGeneration)
		})
	}
}

func TestRuntimeCommandC1LegacyTextAndUUIDCompatibility(t *testing.T) {
	c := c1Command()
	c.Key = "ABCDEF00-0000-1000-8000-000000000001"
	c.ProposedRunID = "ABCDEF00-0000-1000-8000-000000000002"
	// Original actor names allow empty/space text; identity and target text do not.
	c.Actor.Username = ""
	c.Actor.DisplayName = new("")
	got := c1Both(t, c)
	require.Equal(t, strings.ToLower(c.Key), got[0].command.Key)
	for _, canonical := range got {
		require.Equal(t, strings.ToLower(c.ProposedRunID), canonical.command.ProposedRunID)
	}
	c.Actor.Username = " Operator "
	c.Actor.DisplayName = new(" Display ")
	got = c1Both(t, c)
	for _, canonical := range got {
		require.Equal(t, " Operator ", canonical.command.Actor.Username)
		require.Equal(t, " Display ", *canonical.command.Actor.DisplayName)
	}
	for _, text := range []string{" example", "example ", " ", ""} {
		c := c1Command()
		c.Target.ID = text
		c1InvalidBoth(t, c)
	}
	// Limits count UTF-8 bytes, not Unicode code points.
	c = c1Command()
	c.Target.ID = strings.Repeat("é", 128)
	c1Both(t, c)
	c.Target.ID += "a"
	c1InvalidBoth(t, c)
	for _, environment := range []string{"", "example-\xff", " example", strings.Repeat("a", 257)} {
		c := c1Command()
		c.Store.EnvironmentID = environment
		got, err := runtimeCanonicalizeInternal(c)
		require.ErrorIs(t, err, errRuntimeInvalidInternal)
		require.Equal(t, runtimeCanonicalCommandInternal{}, got)
	}
	c = c1Command()
	c.Operation = RuntimeRestartProject
	c.Options.RestartServices = make([]string, 257)
	for i := range c.Options.RestartServices {
		c.Options.RestartServices[i] = "api"
	}
	c1InvalidBoth(t, c) // The accepted limit is checked before set compaction.
}
