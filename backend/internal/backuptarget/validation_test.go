// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original table-driven target grammar tests.

package backuptarget

import (
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTargetInputGrammar(t *testing.T) {
	local := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/backups"}, Name: "target"}
	smb := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "backups", Path: "snapshots"}, Name: "target", Username: "operator", Secret: new("synthetic-password")}
	nfs := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "nfs", Server: "192.0.2.1", Share: "/exports/backups"}, Name: "target"}
	for _, input := range []dto.TargetInput{local, smb, nfs} {
		_, err := Validate(input)
		require.NoError(t, err)
	}
	cases := []struct {
		name   string
		input  dto.TargetInput
		change func(*dto.TargetInput)
	}{
		{"root", local, func(i *dto.TargetInput) { i.Path = "/" }},
		{"relative-local", local, func(i *dto.TargetInput) { i.Path = "srv/backups" }},
		{"parent-local", local, func(i *dto.TargetInput) { i.Path = "/srv/../backups" }},
		{"protected", local, func(i *dto.TargetInput) { i.Path = "/var/lib/docker/volumes" }},
		{"missing-host", local, func(i *dto.TargetInput) { i.HostID = nil }},
		{"remote-host", smb, func(i *dto.TargetInput) { i.HostID = new("0") }},
		{"server", smb, func(i *dto.TargetInput) { i.Server = "invalid/server" }},
		{"share", smb, func(i *dto.TargetInput) { i.Share = "nested/share" }},
		{"parent-remote", smb, func(i *dto.TargetInput) { i.Path = "snapshots/../other" }},
		{"absolute-remote", smb, func(i *dto.TargetInput) { i.Path = "/snapshots" }},
		{"username-comma", smb, func(i *dto.TargetInput) { i.Username = "user,other" }},
		{"username-equals", smb, func(i *dto.TargetInput) { i.Username = "user=other" }},
		{"password-comma", smb, func(i *dto.TargetInput) { i.Secret = new("synthetic,password") }},
		{"password-equals", smb, func(i *dto.TargetInput) { i.Secret = new("synthetic=password") }},
		{"domain-equals", smb, func(i *dto.TargetInput) { i.Options = "domain=example=other" }},
		{"unknown-option", smb, func(i *dto.TargetInput) { i.Options = "credentials=/etc/credentials" }},
		{"version", smb, func(i *dto.TargetInput) { i.Options = "vers=1.0" }},
		{"mode", smb, func(i *dto.TargetInput) { i.Options = "file_mode=9999" }},
		{"uid", smb, func(i *dto.TargetInput) { i.Options = "uid=-1" }},
		{"duplicate-option", smb, func(i *dto.TargetInput) { i.Options = "uid=1,uid=2" }},
		{"nfs-export", nfs, func(i *dto.TargetInput) { i.Share = "exports/backups" }},
		{"nfs-parent", nfs, func(i *dto.TargetInput) { i.Share = "/exports/../backups" }},
		{"nfs-mode", nfs, func(i *dto.TargetInput) { i.Options = "hard,soft" }},
		{"nfs-version", nfs, func(i *dto.TargetInput) { i.Options = "nfsvers=2" }},
		{"nfs-timeo", nfs, func(i *dto.TargetInput) { i.Options = "timeo=0" }},
		{"key-format", local, func(i *dto.TargetInput) { i.RecoveryKey = "invalid" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.change(&c.input)
			_, err := Validate(c.input)
			code(t, err, "backup.target.invalid")
		})
	}
	for _, options := range []string{"vers=2.0", "vers=2.1", "vers=3.0", "vers=3.02", "vers=3.1.1", "domain=EXAMPLE,seal,uid=1000,gid=1000,file_mode=0600,dir_mode=0700"} {
		smb.Options = options
		_, err := Validate(smb)
		require.NoError(t, err, options)
	}
	for _, options := range []string{"nfsvers=3", "nfsvers=4", "nfsvers=4.1", "nfsvers=4.2", "soft,timeo=600,retrans=2,nolock"} {
		nfs.Options = options
		_, err := Validate(nfs)
		require.NoError(t, err, options)
	}
	nfs.Options = ""
	normalized, err := Validate(nfs)
	require.NoError(t, err)
	require.Equal(t, "hard", normalized.Options)
}
