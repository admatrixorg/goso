// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package memorybench is the SPEC 153 Vietnamese memory bench.
//
// Each case writes Minh An sample notes through store.PutMemory and reads
// them through pipeline.RecallFor, the same write and recall path the gateway
// uses for operator notes and cross-session prompt injection. There is no
// model call. A case passes when the admitted note contains the gold
// fragments, or when a negative case admits nothing.
//
// The case list and the floor live in bench/memory-vi. This package is the
// runner because that path is under gateway/internal. The floor was measured
// on main 9545d08. The test fails when a case that passed there fails, or when
// a case that must return nothing admits an item. A higher score does not fail.
// SQLite always runs. Postgres runs when GOSO_TEST_POSTGRES_DSN (or
// GOSO_TEST_DATABASE_URL) is set, and is required when CI=true.
package memorybench
