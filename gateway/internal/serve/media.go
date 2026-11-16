// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tts"
)

// wireMedia shares one TTS config between /api/tts and the tts tool, and
// lets GOSO_VISION_PROVIDER name a saved provider row.
func wireMedia(st store.StoreIface) *tts.Service {
	svc := tts.New()
	builtin.TTSService = svc
	builtin.ProviderStore = st
	return svc
}
