package hotspot

import "testing"

func TestSelectAccessPoint(t *testing.T) {
	got, err := SelectAccessPoint([]AccessPoint{{Name: "ap0", Index: 3}})
	if err != nil || got.Name != "ap0" || got.Index != 3 {
		t.Fatalf("%+v: %v", got, err)
	}
	if _, err := SelectAccessPoint(nil); err == nil {
		t.Fatal("missing AP accepted")
	}
	if _, err := SelectAccessPoint([]AccessPoint{{Name: "ap0"}, {Name: "ap1"}}); err == nil {
		t.Fatal("ambiguous AP accepted")
	}
}
