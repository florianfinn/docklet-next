// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notifications

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestDeliveryTransportAndSDKWaitAreBounded(t *testing.T) {
	require.Equal(t, 15*time.Second, notificationHTTPClient.Timeout)
	require.Equal(t, 15*time.Second, genericHTTPClient.Timeout)
	require.Equal(t, 15*time.Second, smtpTimeoutFromOptionsInternal(smtpBuildOptions{}))
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	defer close(release)
	go func() {
		done <- waitDelivery(context.Background(), time.Millisecond, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Equal(t, "delivery-unreachable", ClassifyDeliveryError(err))
	case <-time.After(time.Second):
		t.Fatal("SDK wait did not return within its bound")
	}
	require.Equal(t, "delivery-invalid", ClassifyDeliveryError(errors.New("template invalid")))
}

type blockedNotificationTransport struct{ entered chan struct{} }

func (t blockedNotificationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	close(t.entered)
	<-r.Context().Done()
	return nil, r.Context().Err()
}
func TestNativeNotificationTransportHonoursClientBound(t *testing.T) {
	c := *notificationHTTPClient
	require.Equal(t, 15*time.Second, c.Timeout)
	c.Timeout = time.Millisecond
	entered := make(chan struct{})
	c.Transport = blockedNotificationTransport{entered}
	done := make(chan error, 1)
	go func() {
		r, e := c.Get("https://notify.example.com/message")
		if r != nil {
			_ = r.Body.Close()
		}
		done <- e
	}()
	<-entered
	select {
	case e := <-done:
		require.Error(t, e)
		require.Equal(t, "delivery-unreachable", ClassifyDeliveryError(e))
	case <-time.After(time.Second):
		t.Fatal("native transport remained blocked")
	}
}
