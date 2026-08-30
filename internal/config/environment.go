package config

import (
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	envconfig "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

var (
	envLoadOnce sync.Once
	envLoadErr  error
)

type environmentField struct {
	key       string
	fieldType reflect.Type
}

type environmentSchema struct {
	byFieldName map[string]environmentField
	fields      []environmentField
	err         error
}

type environmentValueError struct {
	key    string
	reason string
}

func (e environmentValueError) Error() string {
	return e.key + " " + e.reason
}

func (e environmentValueError) Is(target error) bool {
	wanted, ok := target.(environmentValueError)
	return ok && e.key == wanted.key
}

// LoadEnvironment loads ENV_FILE once. An explicitly configured file must be
// readable and valid; a missing default .env remains optional.
func LoadEnvironment() error {
	envLoadOnce.Do(func() {
		envLoadErr = loadEnvFileOnce()
	})
	return envLoadErr
}

func loadEnvironmentSnapshot() (map[string]string, error) {
	loadErr := LoadEnvironment()
	return envconfig.ToMap(os.Environ()), loadErr
}

func decodeEnvironment[T any](snapshot map[string]string) (T, error) {
	schema := inspectEnvironmentSchema(reflect.TypeFor[T]())
	if schema.err != nil {
		var zero T
		return zero, schema.err
	}
	owned := selectEnvironment(snapshot, schema.fields)
	value, err := envconfig.ParseAsWithOptions[T](envconfig.Options{
		Environment: owned,
		FuncMap: map[reflect.Type]envconfig.ParserFunc{
			reflect.TypeFor[bool]():          parseEnvironmentBool,
			reflect.TypeFor[int]():           parseEnvironmentInt,
			reflect.TypeFor[time.Duration](): parseEnvironmentDuration,
		},
	})
	return value, sanitizeEnvironmentError(err, schema)
}

func inspectEnvironmentSchema(root reflect.Type) environmentSchema {
	schema := environmentSchema{byFieldName: make(map[string]environmentField)}
	collectEnvironmentFields(root, "", &schema)
	return schema
}

func collectEnvironmentFields(root reflect.Type, prefix string, schema *environmentSchema) {
	for root.Kind() == reflect.Pointer {
		root = root.Elem()
	}
	if root.Kind() != reflect.Struct {
		return
	}
	for index := 0; index < root.NumField(); index++ {
		field := root.Field(index)
		key, _, _ := strings.Cut(field.Tag.Get("env"), ",")
		if key != "" && key != "-" {
			metadata := environmentField{
				key:       prefix + key,
				fieldType: field.Type,
			}
			schema.fields = append(schema.fields, metadata)
			if existing, exists := schema.byFieldName[field.Name]; exists && existing.key != metadata.key {
				schema.err = fmt.Errorf("environment schema field %q maps to multiple keys", field.Name)
			} else {
				schema.byFieldName[field.Name] = metadata
			}
		}

		nestedType := field.Type
		for nestedType.Kind() == reflect.Pointer {
			nestedType = nestedType.Elem()
		}
		if nestedType.Kind() == reflect.Struct && nestedType != reflect.TypeFor[time.Duration]() {
			collectEnvironmentFields(nestedType, prefix+field.Tag.Get("envPrefix"), schema)
		}
	}
}

func selectEnvironment(snapshot map[string]string, fields []environmentField) map[string]string {
	owned := make(map[string]string, len(fields))
	for _, field := range fields {
		value, exists := snapshot[field.key]
		if !exists {
			continue
		}
		if isPrimitiveEnvironmentType(field.fieldType) {
			// The previous parsers treated surrounding whitespace as insignificant.
			// Keeping an empty normalized value also lets envDefault retain that
			// behavior for variables containing only whitespace.
			value = strings.TrimSpace(value)
		}
		owned[field.key] = value
	}
	return owned
}

func isPrimitiveEnvironmentType(fieldType reflect.Type) bool {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	return fieldType == reflect.TypeFor[bool]() ||
		fieldType == reflect.TypeFor[int]() ||
		fieldType == reflect.TypeFor[time.Duration]()
}

func parseEnvironmentBool(raw string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("must be a boolean")
	}
}

func parseEnvironmentInt(raw string) (any, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, errors.New("must be an integer")
	}
	return value, nil
}

func parseEnvironmentDuration(raw string) (any, error) {
	value := strings.TrimSpace(raw)
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration, nil
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 &&
		int64(seconds) <= math.MaxInt64/int64(time.Second) {
		return time.Duration(seconds) * time.Second, nil
	}
	return time.Duration(0), errors.New("must be a positive duration or number of seconds")
}

func sanitizeEnvironmentError(err error, schema environmentSchema) error {
	if err == nil {
		return nil
	}
	var aggregate envconfig.AggregateError
	if errors.As(err, &aggregate) {
		var sanitized []error
		for _, child := range aggregate.Errors {
			sanitized = appendError(sanitized, sanitizeEnvironmentError(child, schema))
		}
		return errors.Join(sanitized...)
	}

	var parseErr envconfig.ParseError
	if errors.As(err, &parseErr) {
		field, exists := schema.byFieldName[parseErr.Name]
		if !exists {
			return errors.New("environment configuration contains an invalid value")
		}
		return environmentValueError{key: field.key, reason: environmentParseReason(parseErr.Type)}
	}

	var missingErr envconfig.VarIsNotSetError
	if errors.As(err, &missingErr) {
		return environmentValueError{key: missingErr.Key, reason: "is required"}
	}
	var emptyErr envconfig.EmptyVarError
	if errors.As(err, &emptyErr) {
		return environmentValueError{key: emptyErr.Key, reason: "must not be empty"}
	}
	return errors.New("environment configuration is invalid")
}

func environmentParseReason(fieldType reflect.Type) string {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch fieldType {
	case reflect.TypeFor[bool]():
		return "must be a boolean"
	case reflect.TypeFor[int]():
		return "must be an integer"
	case reflect.TypeFor[time.Duration]():
		return "must be a positive duration or number of seconds"
	default:
		return "has an invalid value"
	}
}

func environmentKeyFailed(err error, key string) bool {
	return errors.Is(err, environmentValueError{key: key})
}

func loadEnvFileOnce() error {
	explicitPath := strings.TrimSpace(os.Getenv("ENV_FILE"))
	envFile := explicitPath
	if envFile == "" {
		envFile = ".env"
	}
	err := godotenv.Load(envFile)
	if err == nil {
		return nil
	}
	if explicitPath == "" && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// godotenv parse errors can embed the malformed line and every following
	// line. Never propagate that third-party text because later lines may hold
	// credentials that would then be written by startup diagnostics.
	return fmt.Errorf("load ENV_FILE %q: file is missing, unreadable, or malformed", envFile)
}

func resetEnvLoaderForTest() {
	envLoadOnce = sync.Once{}
	envLoadErr = nil
}
