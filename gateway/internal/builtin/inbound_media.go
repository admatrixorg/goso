// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/skill"
)

// Helpers for channel adapters that receive files from customers. A received
// file lands in the tenant's workspace under UploadsDir, where the agent's
// file tools (read_file, read_document, read_image, read_audio) can open it.

// UploadsDir is the workspace folder for files received on a channel.
const UploadsDir = ".uploads"

// WorkspaceReady reports whether GOSO_WORKSPACE is set, so received files
// can be saved.
func WorkspaceReady() bool { return workspaceConfigured() }

// STTReady reports whether speech-to-text is configured (GOSO_STT_URL).
func STTReady() bool { return sttConfigured() }

// SniffImage returns the MIME type and extension of png, jpeg, gif or webp
// bytes. It reads the bytes, never a name or a header.
func SniffImage(b []byte) (mime, ext string, ok bool) { return sniffImage(b) }

// SniffAudio returns the MIME type and extension of an audio container the
// speech-to-text endpoint accepts.
func SniffAudio(b []byte) (mime, ext string, ok bool) { return sniffAudio(b) }

// SaveUpload writes one received file into tenant's workspace under
// UploadsDir and returns its workspace-relative path. name is the file name
// the customer's app gave (it only shapes the stored name); ext is the
// extension to use, with its dot. It never overwrites or follows links.
func SaveUpload(ctx context.Context, tenant, name, ext string, data []byte) (string, error) {
	if !workspaceConfigured() {
		return "", errNotConfigured
	}
	stem := strings.TrimSuffix(filepath.Base(strings.TrimSpace(name)), filepath.Ext(name))
	if stem == "." || stem == string(filepath.Separator) {
		stem = ""
	}
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext != "" && (!strings.HasPrefix(ext, ".") || len(ext) > 10 || slugDrop.MatchString(ext[1:])) {
		ext = ""
	}
	return saveUnder(skill.WithTenant(ctx, tenant), UploadsDir, stem, "file", ext, data)
}

// Transcribe turns a voice clip into text with the configured speech-to-text
// endpoint. lang is an ISO-639-1 hint ("" → GOSO_STT_LANGUAGE, else vi).
// Errors are short and never carry the key or the endpoint.
func Transcribe(ctx context.Context, data []byte, lang string) (string, error) {
	if !sttConfigured() {
		return "", errors.New("speech-to-text is not configured")
	}
	if int64(len(data)) > MaxAudioInBytes {
		return "", errors.New("too large")
	}
	tr, err := transcribe(ctx, data, lang, "")
	if err != nil {
		return "", err
	}
	return tr.text, nil
}
