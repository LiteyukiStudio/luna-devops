package runtimeconfig

import (
	"reflect"
	"testing"
)

func TestParseLegacyKeyValue(t *testing.T) {
	got, err := ParseLegacyKeyValue(" APP=one\nEMPTY=\nURL=a=b\n# ignored\n")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"APP": "one", "EMPTY": "", "URL": "a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed = %#v, want %#v", got, want)
	}
}

func TestParseLegacyJSONKeyValue(t *testing.T) {
	got, err := ParseLegacyKeyValue(`{"B":"two","A":"one"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "one", "B": "two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed = %#v, want %#v", got, want)
	}
}

func TestParseLegacyKeyValueRejectsDuplicateAndEmptyKey(t *testing.T) {
	for _, value := range []string{"A=one\n A=two", "=value"} {
		if _, err := ParseLegacyKeyValue(value); err == nil {
			t.Fatalf("ParseLegacyKeyValue(%q) succeeded", value)
		}
	}
}

func TestParseLegacyJSONKeyValueNormalizesScalars(t *testing.T) {
	got, err := ParseLegacyKeyValue(`{"PORT":8080,"ENABLED":true,"EMPTY":null}`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"PORT": "8080", "ENABLED": "true", "EMPTY": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed = %#v, want %#v", got, want)
	}
}

func TestEncodeKeyValueNormalizesAndSorts(t *testing.T) {
	got, err := EncodeKeyValue(map[string]string{" B ": "two", "A": "one"})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"A":"one","B":"two"}` {
		t.Fatalf("encoded = %q", got)
	}
}
