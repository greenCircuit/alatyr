package models

// DirectionReason explains why one direction (ingress or egress) of a path
// verdict landed where it did — permitted, or one of the three block classes.
// Lives in models so both the store (which produces it) and the Issue endpoint
// (which surfaces it to the UI) can type it without a cyclic import.
type DirectionReason string

const (
	ReasonPermitted     DirectionReason = "permitted"
	ReasonExplicitDeny  DirectionReason = "explicit-deny"    // has deny to other node
	// An allow rule covers the peer but carves it out (k8s ipBlock.except). No
	// deny object exists to delete — the fixes are widening the allow or dropping
	// the range from the except list, so this can't share ExplicitDeny's wording.
	ReasonCarvedOut     DirectionReason = "carved-out"
	ReasonDefaultDeny   DirectionReason = "default-deny"     // locked, zero allow rules, denyAll policy
	ReasonLockedNoMatch DirectionReason = "locked-no-match"  // locked, allows exist have other rules but not for target
	ReasonNoOpinion     DirectionReason = "no-opinion"
)
