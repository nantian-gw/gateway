package xds

import (
	"testing"
	"time"

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
	if !ok {
		t.Fatal("expected sender to accept response")
	}

	select {
	case err := <-resultCh:
		if err != nil {
			t.Fatalf("unexpected send error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for send result")
	}

	stream.waitForSendCount(t, time.Second)
	responses := stream.snapshotSentResponses()
	if len(responses) != 1 || responses[0].GetVersion() != "v-test" {
		t.Fatalf("unexpected sent responses: %#v", responses)
	}
}

func TestDiscoveryResponseSenderStopsAfterBlockedSendStreamCloses(t *testing.T) {
	t.Parallel()

	stream := newFakeConfigStream()
	stream.blockSend()
	sender := newDiscoveryResponseSender(stream)

	resultCh, ok := sender.send(&controlv1.DiscoveryResponse{Version: "v-blocked"})
	if !ok {
		t.Fatal("expected sender to accept blocked response")
	}
	stream.waitForSendStart(t)

	sender.close()
	stream.release()

	select {
	case err := <-resultCh:
		if status.Code(err) != codes.Canceled {
			t.Fatalf("expected canceled send result after stream close, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocked send to finish")
	}

	select {
	case <-sender.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for sender goroutine to exit")
	}
}
