package mcp

import (
	"reflect"
	"testing"
)

func TestSchemaToMapSimple(t *testing.T) {
	s := Schema{
		Properties: map[string]Property{
			"path":  {Type: "string"},
			"cache": {Type: "string"},
		},
	}

	m := s.ToMap()

	if m["type"] != "object" {
		t.Fatalf("expected type=object, got %v", m["type"])
	}

	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", m["properties"])
	}
	if len(props) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(props))
	}

	pathProp, ok := props["path"].(map[string]any)
	if !ok {
		t.Fatalf("expected path property map, got %T", props["path"])
	}
	if pathProp["type"] != "string" {
		t.Fatalf("expected path type=string, got %v", pathProp["type"])
	}

	if _, exists := m["required"]; exists {
		t.Fatalf("expected no required key, got %v", m["required"])
	}
}

func TestSchemaToMapWithRequired(t *testing.T) {
	s := Schema{
		Properties: map[string]Property{
			"selector": {Type: "string"},
			"new_name": {Type: "string"},
			"path":     {Type: "string"},
		},
		Required: []string{"new_name", "selector"},
	}

	m := s.ToMap()

	required, ok := m["required"].([]string)
	if !ok {
		t.Fatalf("expected required []string, got %T", m["required"])
	}
	expected := []string{"new_name", "selector"}
	if !reflect.DeepEqual(required, expected) {
		t.Fatalf("expected required=%v (sorted), got %v", expected, required)
	}
}

func TestSchemaToMapWithOneOf(t *testing.T) {
	s := Schema{
		Properties: map[string]Property{
			"path": {Type: "string"},
			"capture": {
				OneOf: []Property{
					{Type: "string"},
					{Type: "array", Items: &Property{Type: "string"}},
				},
			},
		},
		Required: []string{"pattern"},
	}

	m := s.ToMap()

	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map, got %T", m["properties"])
	}

	captureProp, ok := props["capture"].(map[string]any)
	if !ok {
		t.Fatalf("expected capture property map, got %T", props["capture"])
	}
	if _, hasType := captureProp["type"]; hasType {
		t.Fatalf("capture should not have type when using oneOf")
	}

	oneOf, ok := captureProp["oneOf"].([]any)
	if !ok {
		t.Fatalf("expected oneOf []any, got %T", captureProp["oneOf"])
	}
	if len(oneOf) != 2 {
		t.Fatalf("expected 2 oneOf entries, got %d", len(oneOf))
	}

	first, ok := oneOf[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first oneOf map, got %T", oneOf[0])
	}
	if first["type"] != "string" {
		t.Fatalf("expected first oneOf type=string, got %v", first["type"])
	}

	second, ok := oneOf[1].(map[string]any)
	if !ok {
		t.Fatalf("expected second oneOf map, got %T", oneOf[1])
	}
	if second["type"] != "array" {
		t.Fatalf("expected second oneOf type=array, got %v", second["type"])
	}

	items, ok := second["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected items map in array oneOf, got %T", second["items"])
	}
	if items["type"] != "string" {
		t.Fatalf("expected items type=string, got %v", items["type"])
	}
}

func TestSchemaToMapWithDescription(t *testing.T) {
	s := Schema{
		Properties: map[string]Property{
			"pattern": {Type: "string", Description: "tree-sitter query pattern"},
		},
	}

	m := s.ToMap()

	props := m["properties"].(map[string]any)
	patternProp := props["pattern"].(map[string]any)

	if patternProp["type"] != "string" {
		t.Fatalf("expected type=string, got %v", patternProp["type"])
	}
	if patternProp["description"] != "tree-sitter query pattern" {
		t.Fatalf("expected description, got %v", patternProp["description"])
	}
}

func TestSchemaToMapEmpty(t *testing.T) {
	s := Schema{}
	m := s.ToMap()

	if m["type"] != "object" {
		t.Fatalf("expected type=object, got %v", m["type"])
	}
	if _, exists := m["properties"]; exists {
		t.Fatalf("expected no properties key for empty schema")
	}
	if _, exists := m["required"]; exists {
		t.Fatalf("expected no required key for empty schema")
	}
}

func TestSchemaRequiredIsSorted(t *testing.T) {
	s := Schema{
		Properties: map[string]Property{
			"z_field": {Type: "string"},
			"a_field": {Type: "string"},
			"m_field": {Type: "string"},
		},
		Required: []string{"z_field", "a_field", "m_field"},
	}

	m := s.ToMap()
	required := m["required"].([]string)
	expected := []string{"a_field", "m_field", "z_field"}
	if !reflect.DeepEqual(required, expected) {
		t.Fatalf("expected sorted required=%v, got %v", expected, required)
	}
}

func TestSchemaToMapDoesNotMutateOriginal(t *testing.T) {
	original := []string{"z", "a"}
	s := Schema{
		Properties: map[string]Property{
			"z": {Type: "string"},
			"a": {Type: "string"},
		},
		Required: original,
	}

	s.ToMap()

	if original[0] != "z" || original[1] != "a" {
		t.Fatalf("ToMap mutated original Required slice: %v", original)
	}
}
