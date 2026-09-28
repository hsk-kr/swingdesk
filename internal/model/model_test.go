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

func TestCategoryAndStanceValid(t *testing.T) {
	for _, c := range categories {
		if !c.Valid() {
			t.Errorf("%q should be valid", c)
		}
	}
	if Category("sports").Valid() || Category("").Valid() {
		t.Error("unexpected valid category")
	}
	for _, s := range stances {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if Stance("flat").Valid() {
		t.Error("unexpected valid stance")
	}
}
