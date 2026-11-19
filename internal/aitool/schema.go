package aitool

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// SchemaForInput generates a deterministic JSON Schema Draft 2020-12 object for a
// Go input struct type T (with additionalProperties: false at every struct level)
// and collects sensitivePaths declared via `sensitive:"true"`.
//
// Supported struct tags:
//   - json:"fieldName,omitempty"
//   - required:"true" (overrides omitempty / pointer optionality)
//   - optional:"true" (marks value field optional even without omitempty)
//   - desc:"human/model description"
//   - enum:"val1|val2|val3"
//   - default:"value"
//   - min:"1" (minimum / minLength / minItems)
//   - max:"50" (maximum / maxLength / maxItems)
//   - pattern:"^[a-z0-9-]+$"
//   - format:"uri"
//   - sensitive:"true" (marks scalar field writeOnly; on map[string]T marks value schema writeOnly and emits field.*)
//   - nullable:"true"
func SchemaForInput[T any]() (map[string]any, []string) {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}, nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("aitool.SchemaForInput requires a struct type, got %s", t.String()))
	}
	var sensitive []string
	schema := buildStructSchema(t, "", true, &sensitive)
	sensitive = dedupeSortedStrings(sensitive)
	return schema, sensitive
}

// SchemaForOutput generates a JSON Schema object for a tool's output type T.
// When T is any or map[string]any, a permissive {"type": "object"} schema is returned.
func SchemaForOutput[T any]() map[string]any {
	t := reflect.TypeFor[T]()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Interface || (t.Kind() == reflect.Map && t.Elem().Kind() == reflect.Interface) {
		return map[string]any{"type": "object"}
	}
	if t.Kind() == reflect.Struct {
		var ignored []string
		return buildStructSchema(t, "", false, &ignored)
	}
	var ignored []string
	return buildTypeSchema(t, reflect.StructField{}, "", false, &ignored)
}

func buildStructSchema(t reflect.Type, prefix string, strictAdditional bool, sensitive *[]string) map[string]any {
	properties := make(map[string]any)
	var required []string

	collectStructFields(t, prefix, strictAdditional, properties, &required, sensitive)

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if strictAdditional {
		schema["additionalProperties"] = false
	}
	if len(required) > 0 {
		required = dedupeSortedStrings(required)
		reqAny := make([]any, len(required))
		for i, name := range required {
			reqAny[i] = name
		}
		schema["required"] = reqAny
	} else if strictAdditional {
		schema["required"] = []any{}
	}
	return schema
}

func collectStructFields(
	t reflect.Type,
	prefix string,
	strictAdditional bool,
	properties map[string]any,
	required *[]string,
	sensitive *[]string,
) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		jsonName, hasOmitEmpty := parseJSONTag(field.Name, jsonTag)
		if field.Anonymous && (jsonTag == "" || strings.HasPrefix(jsonTag, ",")) {
			ft := field.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				collectStructFields(ft, prefix, strictAdditional, properties, required, sensitive)
				continue
			}
		}

		fieldPath := jsonName
		if prefix != "" {
			fieldPath = prefix + "." + jsonName
		}

		isReq := isFieldRequired(field, hasOmitEmpty)
		if isReq {
			*required = append(*required, jsonName)
		}

		propSchema := buildTypeSchema(field.Type, field, fieldPath, strictAdditional, sensitive)
		applyFieldTags(propSchema, field.Type, field, fieldPath, sensitive)
		properties[jsonName] = propSchema
	}
}

func buildTypeSchema(
	t reflect.Type,
	field reflect.StructField,
	fieldPath string,
	strictAdditional bool,
	sensitive *[]string,
) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	isSensitive := strings.EqualFold(strings.TrimSpace(field.Tag.Get("sensitive")), "true")

	switch t.Kind() {
	case reflect.String:
		s := map[string]any{"type": "string"}
		if isSensitive && fieldPath != "" {
			s["writeOnly"] = true
			s["x-luna-sensitive"] = true
			*sensitive = append(*sensitive, fieldPath)
		}
		return s

	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Slice, reflect.Array:
		elemType := t.Elem()
		for elemType.Kind() == reflect.Pointer {
			elemType = elemType.Elem()
		}
		elemPath := fieldPath + ".*"
		if fieldPath == "" {
			elemPath = "*"
		}
		var elemSchema map[string]any
		if elemType.Kind() == reflect.Struct {
			elemSchema = buildStructSchema(elemType, elemPath, strictAdditional, sensitive)
		} else {
			elemSchema = buildTypeSchema(elemType, reflect.StructField{}, elemPath, strictAdditional, sensitive)
			if isSensitive && elemPath != "" {
				elemSchema["writeOnly"] = true
				elemSchema["x-luna-sensitive"] = true
				*sensitive = append(*sensitive, elemPath)
			}
		}
		return map[string]any{
			"type":  "array",
			"items": elemSchema,
		}

	case reflect.Map:
		valType := t.Elem()
		for valType.Kind() == reflect.Pointer {
			valType = valType.Elem()
		}
		if valType.Kind() == reflect.Interface {
			if isSensitive && fieldPath != "" {
				*sensitive = append(*sensitive, fieldPath+".*")
				return map[string]any{
					"type": "object",
					"additionalProperties": map[string]any{
						"writeOnly":        true,
						"x-luna-sensitive": true,
					},
				}
			}
			return map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			}
		}
		valPath := fieldPath + ".*"
		if fieldPath == "" {
			valPath = "*"
		}
		var valSchema map[string]any
		if valType.Kind() == reflect.Struct {
			valSchema = buildStructSchema(valType, valPath, strictAdditional, sensitive)
		} else {
			valSchema = buildTypeSchema(valType, reflect.StructField{}, valPath, strictAdditional, sensitive)
		}
		if isSensitive && valPath != "" {
			valSchema["writeOnly"] = true
			valSchema["x-luna-sensitive"] = true
			*sensitive = append(*sensitive, valPath)
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": valSchema,
		}

	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return map[string]any{"type": "string", "format": "date-time"}
		}
		return buildStructSchema(t, fieldPath, strictAdditional, sensitive)

	default:
		return map[string]any{"type": "object", "additionalProperties": true}
	}
}

func applyFieldTags(schema map[string]any, rawType reflect.Type, field reflect.StructField, _ string, _ *[]string) {
	t := rawType
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if desc := strings.TrimSpace(field.Tag.Get("desc")); desc != "" {
		schema["description"] = desc
	}
	if format := strings.TrimSpace(field.Tag.Get("format")); format != "" {
		schema["format"] = format
	}
	if pattern := strings.TrimSpace(field.Tag.Get("pattern")); pattern != "" {
		schema["pattern"] = pattern
	}
	if enumRaw := strings.TrimSpace(field.Tag.Get("enum")); enumRaw != "" {
		parts := strings.Split(enumRaw, "|")
		values := make([]any, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				values = append(values, p)
			}
		}
		if len(values) > 0 {
			if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
				if items, ok := schema["items"].(map[string]any); ok {
					items["enum"] = values
				}
			} else {
				schema["enum"] = values
			}
		}
	}
	if defRaw, hasDef := field.Tag.Lookup("default"); hasDef && defRaw != "" {
		if parsed, ok := parseScalarTagValue(t, defRaw); ok {
			schema["default"] = parsed
		}
	}
	if minRaw := strings.TrimSpace(field.Tag.Get("min")); minRaw != "" {
		applyBoundTag(schema, t, minRaw, true)
	}
	if maxRaw := strings.TrimSpace(field.Tag.Get("max")); maxRaw != "" {
		applyBoundTag(schema, t, maxRaw, false)
	}
	if strings.EqualFold(strings.TrimSpace(field.Tag.Get("nullable")), "true") {
		applyNullableSchema(schema)
	}
}

func applyBoundTag(schema map[string]any, t reflect.Type, raw string, isMin bool) {
	switch t.Kind() {
	case reflect.String:
		if v, err := strconv.Atoi(raw); err == nil {
			if isMin {
				schema["minLength"] = v
			} else {
				schema["maxLength"] = v
			}
		}
	case reflect.Slice, reflect.Array:
		if v, err := strconv.Atoi(raw); err == nil {
			if isMin {
				schema["minItems"] = v
			} else {
				schema["maxItems"] = v
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			if isMin {
				schema["minimum"] = v
			} else {
				schema["maximum"] = v
			}
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			if isMin {
				schema["minimum"] = v
			} else {
				schema["maximum"] = v
			}
		}
	}
}

func parseScalarTagValue(t reflect.Type, raw string) (any, bool) {
	switch t.Kind() {
	case reflect.String:
		return raw, true
	case reflect.Bool:
		v, err := strconv.ParseBool(raw)
		return v, err == nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseInt(raw, 10, 64)
		return v, err == nil
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(raw, 64)
		return v, err == nil
	default:
		return nil, false
	}
}

func parseJSONTag(fieldName, tag string) (string, bool) {
	if tag == "" {
		return lowerFirst(fieldName), false
	}
	parts := strings.Split(tag, ",")
	name := strings.TrimSpace(parts[0])
	if name == "" {
		name = lowerFirst(fieldName)
	}
	hasOmitEmpty := false
	for _, opt := range parts[1:] {
		if strings.TrimSpace(opt) == "omitempty" {
			hasOmitEmpty = true
			break
		}
	}
	return name, hasOmitEmpty
}

func isFieldRequired(field reflect.StructField, hasOmitEmpty bool) bool {
	if req := strings.TrimSpace(field.Tag.Get("required")); req != "" {
		return strings.EqualFold(req, "true")
	}
	if opt := strings.TrimSpace(field.Tag.Get("optional")); strings.EqualFold(opt, "true") {
		return false
	}
	if hasOmitEmpty {
		return false
	}
	kind := field.Type.Kind()
	if kind == reflect.Pointer || kind == reflect.Slice || kind == reflect.Map {
		return false
	}
	return true
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func dedupeSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return slices.Compact(sorted)
}

func applyNullableSchema(result map[string]any) {
	if enumValues, ok := result["enum"].([]any); ok && len(enumValues) > 0 {
		hasNull := false
		for _, item := range enumValues {
			if item == nil {
				hasNull = true
				break
			}
		}
		if !hasNull {
			result["enum"] = append(append([]any(nil), enumValues...), nil)
		}
	}
	switch typed := result["type"].(type) {
	case string:
		if typed != "" && typed != "null" {
			result["type"] = []any{typed, "null"}
		} else if typed == "" {
			result["type"] = "null"
		}
	case []any:
		for _, item := range typed {
			if item == "null" {
				return
			}
		}
		result["type"] = append(append([]any(nil), typed...), "null")
	case nil:
		if anyOf, ok := result["anyOf"].([]any); ok {
			result["anyOf"] = append(append([]any(nil), anyOf...), map[string]any{"type": "null"})
			return
		}
		if oneOf, ok := result["oneOf"].([]any); ok {
			result["oneOf"] = append(append([]any(nil), oneOf...), map[string]any{"type": "null"})
			return
		}
		if len(result) == 0 {
			result["type"] = []any{"object", "null"}
			return
		}
		branchCopy := make(map[string]any, len(result))
		for key, value := range result {
			branchCopy[key] = value
			delete(result, key)
		}
		result["anyOf"] = []any{branchCopy, map[string]any{"type": "null"}}
	}
}
