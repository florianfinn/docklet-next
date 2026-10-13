// Source: getarcaneapp/arcane/backend/internal/common/api_error_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package common

import (
	"errors"
	"net/http"
	"testing"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
)

func TestToAPIErrorMapsClassifiedValidationDetails(t *testing.T) {
	var err error = &base.FieldError{Field: "name", Err: errors.New("Name is required")}
	err = Classify(ErrValidation, err)

	apiErr := ToAPIError(err)

	require.Equal(t, http.StatusBadRequest, apiErr.HTTPStatus())
	require.Equal(t, APIErrorCodeValidationError, apiErr.Code)
	require.Equal(t, "Name is required", apiErr.Message)
	require.Equal(t, map[string]any{"field": "name"}, apiErr.Details)
}
