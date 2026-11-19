// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import _ "embed"

//go:embed context.json
var contextJSON []byte

//go:embed questions.json
var questionsJSON []byte

//go:embed thresholds.json
var thresholdsJSON []byte

//go:embed testdata/minh-an-relevance.json
var evalCasesJSON []byte
