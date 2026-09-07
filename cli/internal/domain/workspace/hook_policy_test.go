package workspace

import "testing"

func TestCatalogValidatesTrustAcknowledgement(t *testing.T) {
	catalog := HookCatalog{ProjectRoot: "/repo", ProjectTrusted: true}
	if err := catalog.ValidateTrustAcknowledgement("/repo", true); err != nil {
		t.Fatalf("valid trust acknowledgement: %v", err)
	}
	if err := catalog.ValidateTrustAcknowledgement("/other", true); err == nil {
		t.Fatal("accepted trust acknowledgement for another project")
	}
	if err := catalog.ValidateTrustAcknowledgement("/repo", false); err == nil {
		t.Fatal("accepted the opposite trust acknowledgement")
	}
}
