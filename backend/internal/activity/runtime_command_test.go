// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Seven typed input shapes, canonical digest and retained-key regressions.

package activity

import (
	"testing"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
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
