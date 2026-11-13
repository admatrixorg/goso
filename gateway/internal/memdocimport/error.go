// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

// ConfigError is a bad option or a bad input file. The text does not include
// a document body.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }
