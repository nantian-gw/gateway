package xds

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
)

func TestDiscoveryResponseSenderSendsResponse(t *testing.T) {
	t.Parallel()

	stream := newFakeConfigStream()
	sender := newDiscoveryResponseSender(stream)
	defer sender.close()
	defer stream.release()

	resultCh, ok := sender.send(&controlv1.DiscoveryResponse{Version: "v-test"})
	require.True(t, ok, "expected sender to accept response")

	select {
	case err := <-resultCh:
		require.NoError(t, err, "unexpected send error")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for send result")
	}

	stream.waitForSendCount(t, time.Second)
	responses := stream.snapshotSentResponses()
	require.Len(t, responses, 1)
	assert.Equal(t, "v-test", responses[0].GetVersion())
}

func TestDiscoveryResponseSenderStopsAfterBlockedSendStreamCloses(t *testing.T) {
	t.Parallel()

	stream := newFakeConfigStream()
	stream.blockSend()
	sender := newDiscoveryResponseSender(stream)

	resultCh, ok := sender.send(&controlv1.DiscoveryResponse{Version: "v-blocked"})
	require.True(t, ok, "expected sender to accept blocked response")
	stream.waitForSendStart(t)

	sender.close()
	stream.release()

	select {
	case err := <-resultCh:
		assert.Equal(t, codes.Canceled, status.Code(err), "expected canceled send result after stream close")
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocked send to finish")
	}

	select {
	case <-sender.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sender goroutine to exit")
	}
}