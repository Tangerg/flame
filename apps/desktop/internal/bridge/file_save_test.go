package bridge

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestDecodeFileSavePreservesExactBinaryAndTextContents(t *testing.T) {
	for _, content := range [][]byte{
		{},
		{0x50, 0x4b, 0, 0xff, 0x80, 13, 10},
		[]byte("Café 火焰 🔥\r\n\tline\x00 end\n"),
	} {
		for _, name := range []string{"财务报告.xlsx", "deck.pptx", "report.pdf", "document.docx", "bundle.zip", "README", ".gitignore"} {
			decoded, err := DecodeFileSave(name, base64.StdEncoding.EncodeToString(content))
			if err != nil || !bytes.Equal(decoded, content) {
				t.Fatalf("DecodeFileSave(%q) = %v, %v; want %v unchanged", name, decoded, err, content)
			}
		}
	}
}

func TestDecodeFileSaveRefusesInvalidNamesAndNoncanonicalEncoding(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../report.pdf", "dir/report.docx", `dir\report.xlsx`, "C:report.pptx", "report\n.pdf", "report\x00.pdf", "report\u0085.pdf", "report\xff.pdf"} {
		if _, err := DecodeFileSave(name, "AA=="); err == nil {
			t.Errorf("invalid filename %q was accepted", name)
		}
	}
	for _, encoded := range []string{"not base64", "AA", "AB==", "AA==\n", "AA====", "AA-_"} {
		if _, err := DecodeFileSave("report.pdf", encoded); err == nil {
			t.Errorf("invalid content encoding %q was accepted", encoded)
		}
	}
}
