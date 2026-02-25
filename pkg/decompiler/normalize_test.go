package decompiler

import (
	"strings"
	"testing"
)

func TestStripGhidraWarnings(t *testing.T) {
	input := []byte(`/* WARNING: Unknown calling convention */
void FUN_00401000(void) {
  /* WARNING: Subroutine does not return */
  exit(0);
}`)
	result := NormalizeGhidra(input)
	if strings.Contains(string(result), "WARNING") {
		t.Fatalf("expected WARNING comments stripped, got:\n%s", result)
	}
	if !strings.Contains(string(result), "exit(0)") {
		t.Fatal("expected function body preserved")
	}
}

func TestStripGhidraLineWarnings(t *testing.T) {
	input := []byte(`void f(void) {
  // WARNING: This is a line warning
  return;
}`)
	result := NormalizeGhidra(input)
	if strings.Contains(string(result), "WARNING") {
		t.Fatalf("expected line WARNING stripped, got:\n%s", result)
	}
}

func TestNormalizeUndefinedTypes(t *testing.T) {
	input := []byte("undefined4 x = 0;\nundefined8 y = 0;\nundefined z;")
	result := NormalizeGhidra(input)
	s := string(result)
	if !strings.Contains(s, "uint32_t x") {
		t.Fatal("expected undefined4 -> uint32_t")
	}
	if !strings.Contains(s, "uint64_t y") {
		t.Fatal("expected undefined8 -> uint64_t")
	}
	if !strings.Contains(s, "void* z") {
		t.Fatal("expected undefined -> void*")
	}
}

func TestPreserveFunctionBodies(t *testing.T) {
	input := []byte(`/* WARNING: skip */
void process(undefined4 param_1) {
  undefined4 local_c;
  local_c = param_1 + 1;
  FUN_00401000(local_c);
}`)
	result := NormalizeGhidra(input)
	s := string(result)
	if !strings.Contains(s, "FUN_00401000") {
		t.Fatal("expected call site preserved")
	}
	if !strings.Contains(s, "uint32_t param_1") {
		t.Fatal("expected param type normalized")
	}
}

func TestNormalizeIDA(t *testing.T) {
	input := []byte("__int64 result; _DWORD *ptr; __int8 flag;")
	result := NormalizeIDA(input)
	s := string(result)
	if !strings.Contains(s, "int64_t result") {
		t.Fatal("expected __int64 -> int64_t")
	}
	if !strings.Contains(s, "uint32_t *ptr") {
		t.Fatal("expected _DWORD -> uint32_t")
	}
	if !strings.Contains(s, "int8_t flag") {
		t.Fatal("expected __int8 -> int8_t")
	}
}
