package action

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestNewCtxDefaults(t *testing.T) {
	ctx := NewCtx(context.Background())
	if ctx.Output() == nil {
		t.Fatal("Output() must never be nil")
	}
	if ctx.Output() != io.Discard {
		t.Fatal("default Output() should be io.Discard")
	}
	// Emit should not panic with no handler.
	ctx.Emit(StatusEvent{Progress: 0.5, Message: "ok"})
}

func TestNewCtxNilContext(t *testing.T) {
	ctx := NewCtx(nil)
	if ctx == nil {
		t.Fatal("NewCtx(nil) must not return nil")
	}
}

func TestNewCtxWithOutput(t *testing.T) {
	var buf bytes.Buffer
	ctx := NewCtx(context.Background(), WithOutput(&buf))
	n, err := ctx.Output().Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("expected 5 bytes written, got %d", n)
	}
	if buf.String() != "hello" {
		t.Fatalf("expected %q, got %q", "hello", buf.String())
	}
}

func TestNewCtxWithEmit(t *testing.T) {
	var received []Event
	ctx := NewCtx(context.Background(), WithEmit(func(e Event) {
		received = append(received, e)
	}))
	ctx.Emit(StatusEvent{Progress: 1.0, Message: "done"})
	ctx.Emit(OutputEvent{Stream: "stdout", Chunk: []byte("data")})
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if s, ok := received[0].(StatusEvent); !ok || s.Message != "done" {
		t.Fatalf("unexpected first event: %v", received[0])
	}
	if o, ok := received[1].(OutputEvent); !ok || string(o.Chunk) != "data" {
		t.Fatalf("unexpected second event: %v", received[1])
	}
}

func TestBaseCtxSatisfiesInterface(t *testing.T) {
	var ctx Ctx = BaseCtx{Context: context.Background()}
	if ctx.Output() != io.Discard {
		t.Fatal("BaseCtx.Output() should be io.Discard")
	}
	// Must not panic.
	ctx.Emit(StatusEvent{})
}
