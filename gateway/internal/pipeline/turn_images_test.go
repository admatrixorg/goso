// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

type imageFake struct{ ok bool }

func (imageFake) Name() string                                        { return "image-fake" }
func (imageFake) Chat(context.Context, []llm.Message) (string, error) { return "", nil }
func (f imageFake) AcceptsImages() bool                               { return f.ok }

func TestAttachTurnImages(t *testing.T) {
	img := llm.ImagePart{MIME: "image/png", Data: []byte{1, 2, 3}}
	msgs := []llm.Message{{Role: "user", Content: "trước"}, {Role: "assistant", Content: "ok"}, {Role: "user", Content: "<media:image> xem giúp"}, {Role: "system", Content: "s"}}
	ctx := WithTurnImages(context.Background(), []llm.ImagePart{img})

	got := AttachTurnImages(ctx, imageFake{ok: true}, msgs)
	if len(got[2].Images) != 1 || len(got[0].Images) != 0 {
		t.Fatalf("got %+v", got)
	}
	if len(msgs[2].Images) != 0 {
		t.Fatal("input slice was changed")
	}
	if out := AttachTurnImages(ctx, imageFake{ok: false}, msgs); len(out[2].Images) != 0 {
		t.Fatal("provider without image input got images")
	}
	if out := AttachTurnImages(ctx, llm.Echo{}, msgs); len(out[2].Images) != 0 {
		t.Fatal("echo got images")
	}
	if out := AttachTurnImages(context.Background(), imageFake{ok: true}, msgs); len(out[2].Images) != 0 {
		t.Fatal("no images on ctx")
	}
}
