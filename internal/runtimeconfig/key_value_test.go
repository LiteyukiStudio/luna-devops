package runtimeconfig

import (
	"reflect"
	"testing"
)

func TestDecodeKeyValue(t *testing.T) {
	got, err := DecodeKeyValue(`{"APP":"one","EMPTY":"","URL":"a=b"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"APP": "one", "EMPTY": "", "URL": "a=b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed = %#v, want %#v", got, want)
	}
}

func TestDecodeJSONKeyValue(t *testing.T) {
	got, err := DecodeKeyValue(`{"B":"two","A":"one"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "one", "B": "two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed = %#v, want %#v", got, want)
	}
}

func TestDecodeKeyValueRejectsLegacyAndInvalidFormats(t *testing.T) {
	for _, value := range []string{"A=one", `{"":"value"}`, `{"PORT":8080}`, `null`, `[]`} {
		if _, err := DecodeKeyValue(value); err == nil {
			t.Fatalf("DecodeKeyValue(%q) succeeded", value)
		}
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
