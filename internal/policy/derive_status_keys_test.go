package policy

import (
	"testing"

	"graph/internal/models"
)

// Covers: new HasL7 → StatusL7Applied derivation branch added for istio L7.
// Same test exercises the absence path so no separate negative test needed.
func TestDeriveStatusKeys_L7AppliedBranch(t *testing.T) {
	withL7 := DeriveStatusKeys(models.PolicyStatus{HasL7: true})
	if !contains(withL7, models.StatusL7Applied) {
		t.Errorf("HasL7=true → missing StatusL7Applied; got %v", withL7)
	}

	withoutL7 := DeriveStatusKeys(models.PolicyStatus{})
	if contains(withoutL7, models.StatusL7Applied) {
		t.Errorf("HasL7=false → unexpected StatusL7Applied; got %v", withoutL7)
	}
}

// Covers: HasL7 OR semantics across engines — any engine asserting L7
// should make the effective status reflect it.
func TestIntersectPolicyStatus_HasL7Ored(t *testing.T) {
	result := IntersectPolicyStatus([]models.PolicyStatus{
		{HasL7: true},
		{HasL7: false},
	})
	if !result.HasL7 {
		t.Errorf("HasL7 across engines = false, want true (OR semantics)")
	}
}

func contains(keys []models.StatusKey, needle models.StatusKey) bool {
	for _, key := range keys {
		if key == needle {
			return true
		}
	}
	return false
}
