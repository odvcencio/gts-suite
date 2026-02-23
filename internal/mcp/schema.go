package mcp

import "sort"

// Schema builds a JSON Schema object for MCP tool inputs.
type Schema struct {
	Properties map[string]Property
	Required   []string
}

// Property describes a single schema property.
type Property struct {
	Type        string     `json:"type,omitempty"`
	Description string     `json:"description,omitempty"`
	Enum        []string   `json:"enum,omitempty"`
	Items       *Property  `json:"items,omitempty"`
	OneOf       []Property `json:"oneOf,omitempty"`
}

// ToMap converts a Schema to the map[string]any format expected by Tool.InputSchema.
func (s Schema) ToMap() map[string]any {
	result := map[string]any{
		"type": "object",
	}

	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for name, prop := range s.Properties {
			props[name] = propertyToMap(prop)
		}
		result["properties"] = props
	}

	if len(s.Required) > 0 {
		sorted := make([]string, len(s.Required))
		copy(sorted, s.Required)
		sort.Strings(sorted)
		result["required"] = sorted
	}

	return result
}

func propertyToMap(p Property) map[string]any {
	m := make(map[string]any)
	if p.Type != "" {
		m["type"] = p.Type
	}
	if p.Description != "" {
		m["description"] = p.Description
	}
	if len(p.Enum) > 0 {
		m["enum"] = p.Enum
	}
	if p.Items != nil {
		m["items"] = propertyToMap(*p.Items)
	}
	if len(p.OneOf) > 0 {
		oneOf := make([]any, len(p.OneOf))
		for i, o := range p.OneOf {
			oneOf[i] = propertyToMap(o)
		}
		m["oneOf"] = oneOf
	}
	return m
}
