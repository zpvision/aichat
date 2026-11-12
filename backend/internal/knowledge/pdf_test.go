package knowledge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func pdfFixture(pages []string) []byte {
	objects := []string{"<</Type/Catalog/Pages 2 0 R>>", "", "<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>"}
	kids := []string{}
	for i, text := range pages {
		page := 4 + i*2
		stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
		objects = append(objects, fmt.Sprintf("<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Resources<</Font<</F1 3 0 R>>>>/Contents %d 0 R>>", page+1), fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(stream), stream))
	}
	objects[1] = fmt.Sprintf("<</Type/Pages/Kids[%s]/Count %d>>", strings.Join(kids, " "), len(pages))
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, o := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer<</Size %d/Root 1 0 R>>\nstartxref\n%d\n%%%%EOF", len(offsets), xref)
	return b.Bytes()
}
func TestPDFPagesAndOCR(t *testing.T) {
	if os.Getenv("PDF_TESTS") != "1" {
		t.Skip("PDF_TESTS=1 requires built extractor image")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pages, e := Extract(ctx, pdfFixture([]string{"First page", "Second page"}), "pdf")
	if e != nil || len(pages) != 2 || pages[1].Number != 2 || !strings.Contains(pages[1].Text, "Second page") {
		t.Fatal(pages, e)
	}
	if _, e = Extract(ctx, pdfFixture([]string{""}), "pdf"); e == nil || e.Error() != "ocr_required" {
		t.Fatal("OCR status", e)
	}
	if _, e = Extract(ctx, []byte("%PDF-1.4\nbroken"), "pdf"); e == nil {
		t.Fatal("invalid PDF accepted")
	}
}
