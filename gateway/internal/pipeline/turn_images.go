// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

// A channel turn can carry images the customer sent with the message. They
// ride on the context, not in the stored message: the session keeps only the
// text (with each file's workspace path), and the images go to the model for
// this one turn.

type turnImagesKey struct{}

// WithTurnImages attaches the current message's images to ctx.
func WithTurnImages(ctx context.Context, imgs []llm.ImagePart) context.Context {
	if len(imgs) == 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, turnImagesKey{}, imgs)
}

// TurnImagesFrom returns the images WithTurnImages attached, or nil.
func TurnImagesFrom(ctx context.Context) []llm.ImagePart {
	if ctx == nil {
		return nil
	}
	imgs, _ := ctx.Value(turnImagesKey{}).([]llm.ImagePart)
	return imgs
}

// AttachTurnImages puts the turn's images on the last user message when p
// sends images to the model. Otherwise msgs is returned unchanged and the
// agent works from the file paths in the text (read_image).
func AttachTurnImages(ctx context.Context, p llm.Provider, msgs []llm.Message) []llm.Message {
	imgs := TurnImagesFrom(ctx)
	if len(imgs) == 0 || p == nil || !llm.SupportsImages(p) {
		return msgs
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		out := make([]llm.Message, len(msgs))
		copy(out, msgs)
		out[i].Images = append(append([]llm.ImagePart(nil), out[i].Images...), imgs...)
		return out
	}
	return msgs
}
