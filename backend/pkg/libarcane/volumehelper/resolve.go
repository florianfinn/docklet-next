// Source: getarcaneapp/arcane/backend/pkg/libarcane/volumehelper/helper.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Extracted shared image-resolution port; classified failures, bounded pulls and concrete local image IDs.

package volumehelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// Image is the resolved local tools image; Pulled reports a registry pull.
type Image struct {
	Ref, ID string
	Pulled  bool
}
type ImageUnavailable struct{ Err error }

func (e *ImageUnavailable) Error() string { return "failed to resolve helper image: " + e.Err.Error() }
func (e *ImageUnavailable) Unwrap() error { return e.Err }

type DockerError struct{ Err error }

func (e *DockerError) Error() string { return "tools image Docker error: " + e.Err.Error() }
func (e *DockerError) Unwrap() error { return e.Err }

func unavailable(err error) bool {
	if errdefs.IsNotFound(err) || errdefs.IsUnauthorized(err) || errdefs.IsPermissionDenied(err) {
		return true
	}
	// Network errors at this port describe the connection to the Docker daemon.
	// Registry dial/DNS failures arrive in the daemon's pull response instead.
	var network net.Error
	if client.IsErrConnectionFailed(err) || errors.As(err, &network) {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "pull access denied") || strings.Contains(lower, "manifest unknown") || strings.Contains(lower, `get "http`) && registryUnavailable(lower)
}

// registryUnavailable: the daemon answered, but could not dial or resolve the
// registry (a dial timeout included). Other timeouts stay daemon errors.
func registryUnavailable(message string) bool {
	lower := strings.ToLower(message)
	return strings.Contains(lower, "dial tcp") || strings.Contains(lower, "no such host")
}

// pullFailure classifies the daemon's answer locally and keeps only a neutral
// cause: an authenticated pull's error text may reflect registry credentials.
func pullFailure(err error) error {
	if unavailable(err) {
		return &ImageUnavailable{errors.New("helper image pull refused")}
	}
	return &DockerError{errors.New("helper image pull failed")}
}

// PullOptions supplies registry authentication for a reference; nil pulls anonymously.
type PullOptions func(ctx context.Context, ref string) (client.ImagePullOptions, error)

// Resolve uses the agent's complete configured reference and resolves it to a
// local immutable ID. Daemon failures never imply missing helper capabilities.
func Resolve(ctx context.Context, cli *client.Client, configured string, auth PullOptions) (Image, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if cli == nil {
		return Image{}, &DockerError{errors.New("docker service unavailable")}
	}
	ref := ToolsImage(configured)
	inspected, err := cli.ImageInspect(ctx, ref)
	if err == nil {
		return Image{Ref: ref, ID: inspected.ID}, nil
	}
	if !errdefs.IsNotFound(err) {
		return Image{}, &DockerError{fmt.Errorf("inspect helper image: %w", err)}
	}
	options := client.ImagePullOptions{}
	if auth != nil {
		if options, err = auth(ctx, ref); err != nil {
			return Image{}, &DockerError{errors.New("helper image credentials unavailable")}
		}
	}
	pullReader, err := cli.ImagePull(ctx, ref, options)
	if err != nil {
		if ctx.Err() != nil {
			return Image{}, &DockerError{ctx.Err()}
		}
		return Image{}, pullFailure(err)
	}
	if pullReader == nil {
		return Image{}, &DockerError{errors.New("helper image pull returned no response body")}
	}
	defer pullReader.Close()
	stop := context.AfterFunc(ctx, func() { _ = pullReader.Close() })
	defer stop()
	decoder := json.NewDecoder(pullReader)
	for {
		var progress struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := decoder.Decode(&progress); err == io.EOF {
			break
		} else if err != nil {
			return Image{}, &DockerError{fmt.Errorf("read helper image pull response: %w", err)}
		}
		message := progress.Error + " " + progress.ErrorDetail.Message
		if strings.TrimSpace(message) != "" {
			err := errors.New("helper image pull failed")
			lower := strings.ToLower(message)
			if registryUnavailable(lower) {
				return Image{}, &ImageUnavailable{err}
			}
			if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
				return Image{}, &DockerError{err}
			}
			for _, code := range []string{"not found", "manifest unknown", "denied", "unauthorized"} {
				if strings.Contains(lower, code) {
					return Image{}, &ImageUnavailable{err}
				}
			}
			return Image{}, &DockerError{err}
		}
	}
	inspected, err = cli.ImageInspect(ctx, ref)
	if err != nil {
		return Image{}, &DockerError{fmt.Errorf("inspect pulled helper image: %w", err)}
	}
	return Image{Ref: ref, ID: inspected.ID, Pulled: true}, nil
}
