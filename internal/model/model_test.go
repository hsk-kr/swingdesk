package model

import "testing"

func TestKindValid(t *testing.T) {
	for _, k := range kinds {
		if !k.Valid() {
			t.Errorf("%q should be valid", k)
		}
	}
	for _, k := range []Kind{"", "stock", "EQUITY"} {
		if k.Valid() {
			t.Errorf("%q should be invalid", k)
		}
	}
}
