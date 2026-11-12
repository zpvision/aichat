package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"os/exec"
	"site-assistant/internal/core"
	"strings"
	"time"
	"unicode/utf8"
)

const maxText = 2 << 20

func Extract(ctx context.Context, b []byte, format string) ([]Page, error) {
	if len(b) == 0 {
		return nil, core.Fail("empty_document", 422)
	}
	var pages []Page
	switch format {
	case "txt", "md":
		if !utf8.Valid(b) || bytes.Contains(b, []byte{0}) || bytes.HasPrefix(b, []byte("%PDF-")) || bytes.HasPrefix(b, []byte("PK\x03\x04")) {
			return nil, core.Fail("invalid_file", 422)
		}
		pages = []Page{{Text: strings.TrimPrefix(string(b), "\ufeff")}}
	case "docx":
		z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if e != nil || len(z.File) > 1000 {
			return nil, core.Fail("invalid_file", 422)
		}
		var total uint64
		var doc *zip.File
		types := false
		for _, f := range z.File {
			total += f.UncompressedSize64
			if total > 20<<20 || strings.Contains(f.Name, "..") || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "\\") {
				return nil, core.Fail("invalid_file", 422)
			}
			if f.Name == "word/document.xml" {
				doc = f
			}
			if f.Name == "[Content_Types].xml" {
				types = true
			}
		}
		if doc == nil || !types {
			return nil, core.Fail("invalid_file", 422)
		}
		r, e := doc.Open()
		if e != nil {
			return nil, core.Fail("invalid_file", 422)
		}
		defer r.Close()
		d := xml.NewDecoder(io.LimitReader(r, 20<<20))
		var out strings.Builder
		inText := false
		for {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			tok, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, core.Fail("invalid_file", 422)
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "t" {
					inText = true
				}
				if t.Name.Local == "tab" {
					out.WriteString("\t")
				}
			case xml.CharData:
				if inText {
					out.Write(t)
				}
			case xml.EndElement:
				if t.Name.Local == "t" {
					inText = false
				}
				if t.Name.Local == "p" || t.Name.Local == "tr" {
					out.WriteString("\n\n")
				}
				if t.Name.Local == "tc" {
					out.WriteString("\t")
				}
			}
			if out.Len() > maxText {
				return nil, core.Fail("file_too_large", 413)
			}
		}
		pages = []Page{{Text: out.String()}}
	case "pdf":
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			return nil, core.Fail("invalid_file", 422)
		}
		// Poppler runs as a bounded child process, never through a shell. Original filenames are not used.
		name := "site-assistant-pdf-" + core.ID()
		cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--name", name, "--network", "none", "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--memory", "256m", "--cpus", "1", "--pids-limit", "32", "-i", "site-assistant-extractor:local")
		cmd.Stdin = bytes.NewReader(b)
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
		}()
		var stdout limitedBuffer
		stdout.limit = maxText
		cmd.Stdout = &stdout
		cmd.Stderr = io.Discard
		if e := cmd.Run(); e != nil {
			if stdout.exceeded {
				return nil, core.Fail("file_too_large", 413)
			}
			if _, ok := e.(*exec.Error); ok {
				return nil, core.Fail("extractor_unavailable", 503)
			}
			if exit, ok := e.(*exec.ExitError); ok && exit.ExitCode() == 42 {
				return nil, core.Fail("file_too_large", 413)
			}
			if exit, ok := e.(*exec.ExitError); ok && (exit.ExitCode() == 125 || exit.ExitCode() == 126 || exit.ExitCode() == 127) {
				return nil, core.Fail("extractor_unavailable", 503)
			}
			return nil, core.Fail("invalid_file", 422)
		}
		for i, p := range strings.Split(stdout.String(), "\f") {
			if i >= 500 && strings.TrimSpace(p) != "" {
				return nil, core.Fail("file_too_large", 413)
			}
			if strings.TrimSpace(p) != "" {
				pages = append(pages, Page{Text: p, Number: i + 1})
			}
		}
	default:
		return nil, core.Fail("unsupported_file", 422)
	}
	size := 0
	nonempty := false
	for i := range pages {
		pages[i].Text = strings.TrimSpace(strings.ReplaceAll(pages[i].Text, "\x00", ""))
		size += len(pages[i].Text)
		nonempty = nonempty || pages[i].Text != ""
	}
	if size > maxText {
		return nil, core.Fail("file_too_large", 413)
	}
	if !nonempty {
		code := "empty_document"
		if format == "pdf" {
			code = "ocr_required"
		}
		return nil, core.Fail(code, 422)
	}
	return pages, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.exceeded = true
		return 0, core.Fail("file_too_large", 413)
	}
	return b.Buffer.Write(p)
}
