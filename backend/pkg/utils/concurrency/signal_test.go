// Source: getarcaneapp/arcane/backend/pkg/utils/concurrency/signal_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package concurrency

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignalPublishesOnceAndUnsubscribesInternal(t *testing.T) {
	signal := NewSignal[int]()
	firstCalls := 0
	secondCalls := 0
	unsubscribeFirst := signal.Subscribe(func(value int) {
		firstCalls += value
	})
	signal.Subscribe(func(value int) {
		secondCalls += value
	})

	signal.Publish(1)
	unsubscribeFirst()
	unsubscribeFirst()
	signal.Publish(2)

	require.Equal(t, 1, firstCalls)
	require.Equal(t, 3, secondCalls)
}

func TestSignalContainsSubscriberPanicInternal(t *testing.T) {
	signal := NewSignal[int]()
	called := false
	signal.Subscribe(func(int) { panic("deliberate signal panic") })
	signal.Subscribe(func(int) { called = true })

	signal.Publish(1)

	require.True(t, called)
}
