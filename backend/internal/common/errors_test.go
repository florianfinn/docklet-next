// Source: getarcaneapp/arcane/backend/internal/common/errors_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package common

import (
	"errors"
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
)

func TestClassifyPreservesChainAndField(t *testing.T) {
	cause := errors.New("root cause")
	var err error = &base.FieldError{Field: "name", Err: cause}
	err = Classify(ErrInvalidEnvKey, err)
	err = fmt.Errorf("validate input: %w", err)

	require.EqualError(t, err, "validate input: root cause")
	require.ErrorIs(t, err, ErrInvalidEnvKey)
	require.ErrorIs(t, err, ErrValidation)
	require.ErrorIs(t, err, cause)
	fieldErr, ok := errors.AsType[*base.FieldError](err)
	require.True(t, ok)
	require.Equal(t, "name", fieldErr.Field)
}
