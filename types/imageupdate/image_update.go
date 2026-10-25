// Source: getarcaneapp/arcane/types/imageupdate/image_update.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package imageupdate

import (
	"encoding/json"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"time"

	"github.com/florianfinn/docklet-next/types/containerregistry"
)

type Response struct {
	State     string    `json:"state" enum:"unchecked,current,version,rebuild,local,unknown,error"`
	CheckedAt time.Time `json:"checkedAt"`
	Platform  string    `json:"platform,omitempty"`
	AuthState string    `json:"authState" enum:"anonymous,credential,authentication-required,authentication-failed,unknown"`
	Code      string    `json:"code,omitempty"`

	// ImageUpdate preserves the image-scoped result before container-policy aggregation.
	ImageUpdate *Response `json:"imageUpdate,omitempty"`
	// ImageRef identifies the configured reference for a container-specific check.
	ImageRef string `json:"imageRef,omitempty"`
	// ContainerUpdates contains policy-specific results keyed by container ID.
	ContainerUpdates map[string]*Response `json:"containerUpdates,omitempty"`

	// CheckTime is the time when the update check was performed.
	//
	// Required: true
	CheckTime time.Time `json:"checkTime"`

	// ActivityID is the background activity that tracked this check.
	//
	// Required: false
	ActivityID *string `json:"activityId,omitempty"`

	// UpdateType describes the type of update (e.g., major, minor, patch).
	//
	// Required: true
	UpdateType string `json:"updateType"`

	// CurrentVersion is the current version of the image.
	//
	// Required: true
	CurrentVersion string `json:"currentVersion"`

	// LatestVersion is the latest available version of the image.
	//
	// Required: false
	LatestVersion string `json:"latestVersion,omitempty"`

	// CurrentDigest is the digest (hash) of the current image.
	//
	// Required: false
	CurrentDigest string `json:"currentDigest,omitempty"`

	// LatestDigest is the digest (hash) of the latest available image.
	//
	// Required: false
	LatestDigest string `json:"latestDigest,omitempty"`

	// Error contains any error message from the update check.
	//
	// Required: false
	Error string `json:"error,omitempty"`

	// AuthMethod is the authentication method used ("none" | "anonymous" | "credential" | "unknown").
	//
	// Required: false
	AuthMethod string `json:"authMethod,omitempty"`

	// AuthUsername is the username used for authentication (for credential method).
	//
	// Required: false
	AuthUsername string `json:"-"`

	// AuthRegistry is the registry host used for authentication.
	//
	// Required: false
	AuthRegistry string `json:"authRegistry,omitempty"`

	// ResponseTimeMs is the response time in milliseconds.
	//
	// Required: true
	ResponseTimeMs int `json:"responseTimeMs"`

	// HasUpdate indicates if an update is available for the image.
	//
	// Required: true
	HasUpdate bool `json:"hasUpdate"`

	// UsedCredential indicates if credentials were used for the update check.
	//
	// Required: false
	UsedCredential bool `json:"usedCredential,omitempty"`
}

type Summary struct {
	// TotalImages is the total number of current Docker images considered.
	//
	// Required: true
	TotalImages int `json:"totalImages"`

	// ImagesWithUpdates is the number of images with available updates.
	//
	// Required: true
	ImagesWithUpdates int `json:"imagesWithUpdates"`

	// DigestUpdates is the number of images with digest updates.
	//
	// Required: true
	DigestUpdates int `json:"digestUpdates"`

	// ErrorsCount is the number of errors encountered during the check.
	//
	// Required: true
	ErrorsCount int `json:"errorsCount"`
}

type BatchImageUpdateRequest struct {
	InlineCredentialsProvided bool `json:"-"`
	// ImageRefs is a list of image references to check for updates.
	//
	// Required: true
	ImageRefs []string `json:"imageRefs" binding:"required"`

	// Credentials is rejected legacy input; use saved registry credentials.
	//
	// Required: false
	Credentials []containerregistry.Credential `json:"credentials,omitempty" deprecated:"true" writeOnly:"true"`
}

type CheckAllImagesRequest struct {
	InlineCredentialsProvided bool `json:"-"`
	// Credentials is rejected legacy input; use saved registry credentials.
	//
	// Required: false
	Credentials []containerregistry.Credential `json:"credentials,omitempty" deprecated:"true" writeOnly:"true"`
}

type BatchResponse map[string]*Response

func FromInfo(i *imageDTO.UpdateInfo) *Response {
	return &Response{State: i.State, CheckedAt: i.CheckedAt, ImageRef: i.ImageRef, Platform: i.Platform, AuthState: i.AuthState, Code: i.Code, CheckTime: i.CheckTime, HasUpdate: i.HasUpdate, UpdateType: i.UpdateType, CurrentVersion: i.CurrentVersion, LatestVersion: i.LatestVersion, CurrentDigest: i.CurrentDigest, LatestDigest: i.LatestDigest, Error: i.Error, AuthMethod: i.AuthMethod, UsedCredential: i.UsedCredential}
}

func (p *BatchImageUpdateRequest) UnmarshalJSON(data []byte) error {
	type plain BatchImageUpdateRequest
	var value plain
	if json.Unmarshal(data, &value) != nil {
		return imageDTO.Failure("update.invalid-target", 400)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	*p = BatchImageUpdateRequest(value)
	for key := range fields {
		p.InlineCredentialsProvided = p.InlineCredentialsProvided || imageDTO.InlineCredentialKey(key)
	}
	return nil
}
func (p *CheckAllImagesRequest) UnmarshalJSON(data []byte) error {
	type plain CheckAllImagesRequest
	var value plain
	if json.Unmarshal(data, &value) != nil {
		return imageDTO.Failure("update.invalid-target", 400)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	*p = CheckAllImagesRequest(value)
	for key := range fields {
		p.InlineCredentialsProvided = p.InlineCredentialsProvided || imageDTO.InlineCredentialKey(key)
	}
	return nil
}
