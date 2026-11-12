// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// sniffed is a type decided from file bytes, never from the client Content-Type.
type sniffed struct {
	kind string
	mime string
	ext  string
}

func sniffFile(path string) (sniffed, error) {
	f, err := os.Open(path)
	if err != nil {
		return sniffed{}, err
	}
	defer f.Close()
	head := make([]byte, 16)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return sniffed{}, err
	}
	head = head[:n]
	if bytes.HasPrefix(head, []byte("%PDF")) {
		return sniffed{kind: "pdf", mime: "application/pdf", ext: ".pdf"}, nil
	}
	if bytes.HasPrefix(head, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return sniffed{kind: "png", mime: "image/png", ext: ".png"}, nil
	}
	if bytes.HasPrefix(head, []byte{0xff, 0xd8, 0xff}) {
		return sniffed{kind: "jpeg", mime: "image/jpeg", ext: ".jpg"}, nil
	}
	if bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a")) {
		return sniffed{kind: "gif", mime: "image/gif", ext: ".gif"}, nil
	}
	if len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")) {
		return sniffed{kind: "webp", mime: "image/webp", ext: ".webp"}, nil
	}
	if bytes.HasPrefix(head, []byte("PK")) {
		kind, err := sniffOffice(path)
		if err != nil {
			return sniffed{}, err
		}
		return kind, nil
	}
	ok, err := fileIsUTF8Text(path)
	if err != nil {
		return sniffed{}, err
	}
	if ok {
		return sniffed{kind: "text", mime: "text/plain", ext: ".txt"}, nil
	}
	return sniffed{}, nil
}

func sniffOffice(path string) (sniffed, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return sniffed{}, nil
	}
	defer zr.Close()
	var docx, xlsx, pptx bool
	for _, f := range zr.File {
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		switch name {
		case "word/document.xml":
			docx = true
		case "xl/workbook.xml":
			xlsx = true
		case "ppt/presentation.xml":
			pptx = true
		}
	}
	switch {
	case docx:
		return sniffed{kind: "docx", mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ext: ".docx"}, nil
	case xlsx:
		return sniffed{kind: "xlsx", mime: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ext: ".xlsx"}, nil
	case pptx:
		return sniffed{kind: "pptx", mime: "application/vnd.openxmlformats-officedocument.presentationml.presentation", ext: ".pptx"}, nil
	default:
		return sniffed{}, nil
	}
}

func fileIsUTF8Text(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, uploadChunk)
	var carry []byte
	for {
		n, err := f.Read(buf)
		data := carry
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return false, nil
		}
		i := 0
		for i < len(data) {
			if !utf8.FullRune(data[i:]) {
				break
			}
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError && size == 1 {
				return false, nil
			}
			i += size
		}
		next := make([]byte, len(data)-i)
		copy(next, data[i:])
		carry = next
		if err == io.EOF {
			return len(carry) == 0, nil
		}
		if err != nil {
			return false, err
		}
	}
}

// clientExt is the lower-case extension of the uploaded name, including the dot.
func clientExt(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i <= 0 {
		return ""
	} else {
		return strings.ToLower(name[i:])
	}
}

func unsupportedExt(ext string) bool {
	switch ext {
	case ".doc", ".odt",
		".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".wma",
		".mp4", ".mov", ".avi", ".mkv", ".webm", ".wmv", ".mpeg", ".mpg":
		return true
	default:
		return false
	}
}

// extAgrees reports whether the client extension matches the sniffed type.
// The extension is not used to accept a type the bytes do not support.
func extAgrees(ext string, kind sniffed) (string, bool) {
	if kind.kind == "" {
		return "", false
	}
	switch ext {
	case ".pdf":
		return ".pdf", kind.kind == "pdf"
	case ".docx":
		return ".docx", kind.kind == "docx"
	case ".xlsx":
		return ".xlsx", kind.kind == "xlsx"
	case ".pptx":
		return ".pptx", kind.kind == "pptx"
	case ".png":
		return ".png", kind.kind == "png"
	case ".jpg", ".jpeg":
		return ".jpg", kind.kind == "jpeg"
	case ".gif":
		return ".gif", kind.kind == "gif"
	case ".webp":
		return ".webp", kind.kind == "webp"
	case ".txt":
		return ".txt", kind.kind == "text"
	case ".md":
		return ".md", kind.kind == "text"
	default:
		return "", false
	}
}

func mimeForExt(ext string, kind sniffed) string {
	if ext == ".md" && kind.kind == "text" {
		return "text/markdown"
	}
	if ext == ".txt" && kind.kind == "text" {
		return "text/plain"
	}
	return kind.mime
}
