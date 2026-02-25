package similarity

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	body := "mov eax, [0x401000]\nmov var_1, eax\nmov var_2, [0xDEADBEEF]"
	result := NormalizeBody(body)
	if result != "mov eax, [ADDR] mov v0, eax mov v1, [ADDR]" {
		t.Fatalf("unexpected normalization: %q", result)
	}
}

func TestNormalizeLocalVars(t *testing.T) {
	body := "local_0 = local_1 + var_3\nlocal_0 = local_1"
	result := NormalizeBody(body)
	// local_0 -> v0, local_1 -> v1, var_3 -> v2, then reuse v0, v1
	if result != "v0 = v1 + v2 v0 = v1" {
		t.Fatalf("unexpected: %q", result)
	}
}

func TestExactMatch(t *testing.T) {
	a := "mov eax, [0x401000]\nret"
	b := "mov eax, [0x402000]\nret"
	aNorm := NormalizeBody(a)
	bNorm := NormalizeBody(b)
	if aNorm != bNorm {
		t.Fatalf("expected identical normalized bodies:\n  a=%q\n  b=%q", aNorm, bNorm)
	}
}

func TestFuzzyMatch(t *testing.T) {
	a := "push rbp\nmov rbp, rsp\nmov eax, 1\npop rbp\nret"
	b := "push rbp\nmov rbp, rsp\nmov eax, 2\npop rbp\nret"
	aGrams := Ngrams(NormalizeBody(a), 3)
	bGrams := Ngrams(NormalizeBody(b), 3)
	score := Jaccard(aGrams, bGrams)
	if score < 0.5 {
		t.Fatalf("expected high similarity, got %.2f", score)
	}
}

func TestNoMatch(t *testing.T) {
	a := "push rbp\nmov rbp, rsp\ncall malloc\nret"
	b := "xor eax, eax\nsyscall\nhlt"
	aGrams := Ngrams(NormalizeBody(a), 3)
	bGrams := Ngrams(NormalizeBody(b), 3)
	score := Jaccard(aGrams, bGrams)
	if score >= 0.5 {
		t.Fatalf("expected low similarity, got %.2f", score)
	}
}

func TestJaccardEmpty(t *testing.T) {
	score := Jaccard(nil, nil)
	if score != 0 {
		t.Fatalf("expected 0 for empty sets, got %f", score)
	}
}
