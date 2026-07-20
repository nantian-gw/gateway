package admin

import (
	"testing"
)

func TestLimitedBufferRejectsWritesBeyondLimit(t *testing.T) {
	t.Parallel()

	buffer := newLimitedBuffer(4, errPayloadTooLarge("too large"))
	if _, err := buffer.Write([]byte("ping")); err != nil {
		t.Fatalf("expected first write to fit, got %v", err)
	}
	if string(buffer.Bytes()) != "ping" {
		t.Fatalf("unexpected buffer contents: %q", buffer.Bytes())
	}

	if _, err := buffer.Write([]byte("x")); !isPayloadTooLarge(err) {
		t.Fatalf("expected payload too large error, got %v", err)
	}
}
