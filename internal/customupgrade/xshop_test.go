package customupgrade

import "testing"

func TestXSHOPProductCannotAcceptLegacyCustomOrOfficialPackage(t *testing.T) {
	m, p, k := fixture(t)
	m["product"] = "XSHOP"
	raw, sig := signed(t, m, k)
	if _, err := VerifyManifest(raw, sig, p); err != nil {
		t.Fatalf("XSHOP rejected: %v", err)
	}
	for _, product := range []string{"dujiao-custom", "dujiao-next", "xshop"} {
		m["product"] = product
		raw, sig = signed(t, m, k)
		if _, err := VerifyManifest(raw, sig, p); err == nil {
			t.Fatal("foreign product accepted", product)
		}
	}
}
