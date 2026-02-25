package treesitter

import "testing"

func BenchmarkParserParallelParse(b *testing.B) {
	entry := findEntryByExtension(b, ".go")
	parser, err := NewParser(entry)
	if err != nil {
		b.Fatalf("NewParser returned error: %v", err)
	}

	source := []byte(`package demo

type Service struct{}

func Handle() {
	println("ok")
}
`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			summary, parseErr := parser.Parse("bench.go", source)
			if parseErr != nil {
				b.Fatalf("Parse returned error: %v", parseErr)
			}
			if len(summary.Symbols) == 0 {
				b.Fatal("expected symbols from Parse")
			}
		}
	})
}
