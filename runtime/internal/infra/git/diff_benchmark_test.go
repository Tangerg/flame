package git

import (
	"bytes"
	"runtime"
	"testing"
)

func BenchmarkDiffProjectionAtByteAndRowBudget(b *testing.B) {
	const rows = 4999
	header := []byte("diff --git a/large.txt b/large.txt\nnew file mode 100644\n--- /dev/null\n+++ b/large.txt\n@@ -0,0 +1,4999 @@\n")
	line := bytes.Repeat([]byte{'x'}, ((64<<20)-len(header))/rows)
	line[0], line[len(line)-1] = '+', '\n'
	patch := make([]byte, len(header)+rows*len(line))
	copy(patch, header)
	for offset := len(header); offset < len(patch); offset += len(line) {
		copy(patch[offset:], line)
	}
	for _, projection := range []string{"rows", "summary", "visual"} {
		b.Run(projection, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(patch)))
			for b.Loop() {
				parsed, err := parseUnifiedDiff(patch, 5000, 5000, projection == "rows")
				if err != nil || parsed.truncated || len(parsed.files) != 1 || parsed.end != len(patch) {
					b.Fatalf("budget workload: %+v, %v", parsed, err)
				}
				if projection == "visual" {
					text := string(patch[:parsed.end])
					runtime.KeepAlive(text)
				}
				runtime.KeepAlive(parsed)
			}
		})
	}
}
