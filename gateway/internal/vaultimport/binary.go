// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/vault"
)

// GoClaw registers images, audio, video and office files (pdf, docx, xlsx,
// ...) as vault rows: the bytes stay in the workspace and the row holds the
// path, the type and a short summary built from the file name. It does not
// extract text from them. The importer does the same: the bytes are copied
// to t/<tenant>/<path> unchanged, and the row is a note at <path>.md that
// describes the file. Scoped sync only reads .md and .txt, so the copied
// file never becomes a row of its own and is not served by itself.

// sniffBytes is how much of a file decides text or binary.
const sniffBytes = 8192

// binaryInfo is an attachment found under --src-dir. The bytes are not kept
// in memory; they are streamed again at write time and must still hash to sum.
type binaryInfo struct {
	abs  string
	size int64
	sum  string
	// logical is the copied file's path (the note path without ".md").
	logical string
}

// textExt is an export path the vault serves as text. A binary file there
// is still binary-skipped: its note and its bytes would share one path.
func textExt(p string) bool {
	ext := strings.ToLower(extOf(p))
	return ext == ".md" || ext == ".txt"
}

// sniffBinary reports a file whose first bytes hold a NUL or are not UTF-8.
func sniffBinary(abs string) (bool, error) {
	f, err := os.OpenFile(abs, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	head := make([]byte, sniffBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return true, nil
	}
	if n == sniffBytes {
		// A rune may be cut at the end of the sample.
		for i := 0; i < utf8.UTFMax-1 && len(head) > 0 && !utf8.Valid(head); i++ {
			head = head[:len(head)-1]
		}
	}
	return !utf8.Valid(head), nil
}

// hashFile streams a regular file without following a symlink.
func hashFile(abs string, limit int64) (string, int64, error) {
	f, err := os.OpenFile(abs, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errSymlink
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, limit+1))
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// attachmentCard is the note body for an attachment. It names the file and
// its type the way GoClaw's synthesized summary does, so search by name,
// folder or type finds it. It holds no text taken from inside the file.
func attachmentCard(bin *binaryInfo, docType string) string {
	base := bin.logical
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	ext := strings.TrimPrefix(strings.ToLower(extOf(base)), ".")
	kind := strings.TrimSpace(docType)
	if kind == "" {
		kind = attachmentKind(ext)
	}
	var b strings.Builder
	b.WriteString("Attachment: " + base + "\n")
	b.WriteString("File: " + bin.logical + "\n")
	b.WriteString("Type: " + kind)
	if ext != "" {
		b.WriteString(" (" + ext + ")")
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Size: %d bytes\n", bin.size))
	b.WriteString("SHA-256: " + bin.sum + "\n")
	b.WriteString("Text is not extracted. The file is stored next to this note.\n")
	return b.String()
}

// attachmentKind mirrors GoClaw's media/document split by extension.
func attachmentKind(ext string) string {
	switch ext {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "svg", "heic", "tif", "tiff",
		"mp3", "wav", "ogg", "m4a", "aac", "flac", "opus",
		"mp4", "mov", "avi", "mkv", "webm":
		return "media"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "ods", "odp", "rtf":
		return "document"
	}
	return "file"
}

// publishBinary streams src to t/<tenant>/<logical>. The bytes must still
// hash to sum, so a file that changed during the run is not copied. A
// matching destination is left untouched.
func publishBinary(root, tenant, logical, src, sum string, limit int64) (bool, error) {
	if vault.UnderRaw(logical) {
		return false, errConfig("refusing to write under raw")
	}
	same, err := binaryMatches(root, tenant, logical, sum, limit)
	if err != nil {
		return false, err
	}
	if same {
		return false, nil
	}
	abs, err := resolveVaultFile(root, tenant, logical)
	if err != nil {
		return false, err
	}
	if err := ensureVaultRoot(root); err != nil {
		return false, err
	}
	if err := ensureDir(root, filepath.Dir(abs)); err != nil {
		return false, err
	}
	in, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	defer in.Close()
	tmp := abs + ".import-partial"
	if err := removeRegular(tmp); err != nil {
		return false, err
	}
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return false, err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), io.LimitReader(in, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && hex.EncodeToString(h.Sum(nil)) != sum {
		err = errConfig("source file changed during import")
	}
	if err != nil {
		_ = removeRegular(tmp)
		return false, err
	}
	if info, lerr := os.Lstat(abs); lerr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		_ = removeRegular(tmp)
		return false, errSymlink
	}
	if err := os.Rename(tmp, abs); err != nil {
		_ = removeRegular(tmp)
		return false, err
	}
	return true, nil
}

// binaryMatches reports that t/<tenant>/<logical> already holds bytes with sum.
func binaryMatches(root, tenant, logical, sum string, limit int64) (bool, error) {
	if strings.TrimSpace(root) == "" {
		return true, nil
	}
	abs, err := resolveVaultFile(root, tenant, logical)
	if err != nil {
		return false, err
	}
	if err := walkNoSymlink(root, abs); err != nil {
		return false, err
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, errSymlink
	}
	got, _, err := hashFile(abs, limit)
	if err != nil {
		return false, err
	}
	return got == sum, nil
}

// unpublishFile removes t/<tenant>/<logical> when it is a regular file.
// A missing file is not an error. A symlink is not followed or removed.
func unpublishFile(root, tenant, logical string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	abs, err := resolveVaultFile(root, tenant, logical)
	if err != nil {
		return err
	}
	if err := walkNoSymlink(root, abs); err != nil {
		return err
	}
	return removeRegular(abs)
}

// loadBinary reports an attachment: a non-empty file whose head is binary.
// A text file returns nil with no reason, so the caller reads it as a body.
// Over limit is too-large. The bytes are hashed, not kept.
func loadBinary(abs string, size, limit int64) (*binaryInfo, string, error) {
	if size == 0 {
		return nil, "", nil
	}
	isBin, err := sniffBinary(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "path-missing", nil
	}
	if err != nil {
		return nil, "", err
	}
	if !isBin {
		return nil, "", nil
	}
	if size > limit {
		return nil, reasonTooLarge, nil
	}
	sum, n, err := hashFile(abs, limit)
	if errors.Is(err, errSymlink) {
		return nil, reasonSymlink, nil
	}
	if err != nil {
		return nil, "", err
	}
	if n > limit {
		return nil, reasonTooLarge, nil
	}
	return &binaryInfo{abs: abs, size: n, sum: sum}, "", nil
}

// applyBinary sets the note body for an attachment once its path is placed.
// The marker sum is the version-style checksum, so a later run can tell an
// untouched note from a hand edit.
func (item *keptDoc) applyBinary() {
	meta := scrubMeta(item.src.Metadata, map[string]string{
		"doc_type":     item.src.DocType,
		"scope":        item.src.Scope,
		"custom_scope": item.src.CustomScope,
	})
	plain := composeBody(attachmentCard(item.binary, item.src.DocType), item.src.Summary, meta)
	item.body = stampImport(item.title, plain, strings.TrimSpace(item.src.ID))
	item.sum = hashBody(item.body)
}
