package models

// MeshMembership stamps a workload's mesh participation. Provider/Mode are
// opaque strings so future providers populate the same field.
// Resolved on demand by store.GetWorkloadMesh; never set in the graph payload.
// See docs/arch/0003-istio-mesh-membership-and-mtls.md.
type MeshMembership struct {
	InMesh   bool         `json:"inMesh"`
	Provider string       `json:"provider,omitempty"` // e.g. "istio"
	Mode     string       `json:"mode,omitempty"`     // provider dataplane mode, e.g. "ambient"
	Mtls     *MtlsState   `json:"mtls,omitempty"`     // nil in graph payload; populated by detail endpoint
}


// MtlsState carries the resolved peer-authentication posture for a workload.
// Verdict is the workload-level mode; EffectiveSource points at the PA that
// produced it (nil = installation default applied). PortOverrides covers
// ports that any in-scope PA referenced via portLevelMtls. Sources keeps
// every PA that touched the workload, oldest-first. Issues surfaces
// misconfigurations SRE can act on.
type MtlsState struct {
	Verdict         MeshScope            `json:"verdict"`
	PortOverrides   map[uint32]MeshScope `json:"portOverrides,omitempty"`
	EffectiveSource MtlsPolicyApplied    `json:"effectiveSource,omitempty"`
	Sources         []MtlsSource         `json:"sources,omitempty"`
	Issues          []MtlsIssue		 `json:"issues,omitempty"`
}

// MtlsIssue is one hygiene finding against a workload's PA posture. Refs names
// the PeerAuthentication objects the message is about (e.g. the winner + the
// silently-ignored duplicates) so callers can render clickable culprits
// instead of re-parsing the prose Message.
type MtlsIssue struct {
	Message string      `json:"message"`
	Refs    []PolicyRef `json:"refs,omitempty"`
}

// MtlsPolicyApplied identifies a PeerAuthentication by namespace + name.
type MtlsPolicyApplied struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// MeshVerdict is the per-source mesh-layer reachability outcome for src→dst.
// Verdict is one of "allow" | "deny" | "unknown". EffectiveSource names the
// PA that forced the verdict when applicable.
type MeshVerdict struct {
	Verdict         string             `json:"verdict"`
	Reason          string             `json:"reason"`
	EffectiveSource *MtlsPolicyApplied `json:"effectiveSource,omitempty"`
}



// MtlsSource is one PA entry that touched the workload during the precedence
// walk (oldest-first within each scope).
type MtlsSource struct {
	Namespace   string               `json:"namespace"`
	Name        string               `json:"name"`
	MeshScope   MeshScope            `json:"meshScope"`  // unset, disable, strict, permissive
	MeshSource  MeshSource           `json:"meshSource"` // global, ns, or workload
	PortModes   map[uint32]MeshScope `json:"portModes,omitempty"`
}


type  MeshScope string

const(
	MeshUnset      MeshScope = "unset" // inherit from parent in the PA precedence hierarchy
	MeshDisable    MeshScope = "disable"
	MeshStrict 	   MeshScope = "strict"
	MeshPermissive MeshScope = "permissive"
	MeshUnknown    MeshScope = "unknown"
)

type  MeshSource string

const(
	MeshGlobal    MeshSource = "global"
	MeshNs        MeshSource = "ns"
	MeshWorkload  MeshSource = "workload"
)

// overall metrics of mesh so can display then without doing any filtering
type MeshMetrics struct {
	NsEnrolled        int32 `json:"nsEnrolled"`
    NsPartial         int32 `json:"nsPartial"`
    WorkloadsEnrolled int32 `json:"workloadsEnrolled"`
    MtlsStrict        int32 `json:"mtlsStrict"`
    MtlsPermissive    int32 `json:"mtlsPermissive"`
    MtlsDisabled      int32 `json:"mtlsDisabled"`
    MtlsUnset         int32 `json:"mtlsUnset"`
    MtlsUnknown       int32 `json:"mtlsUnknown"`
}

type MeshBuildResult struct {
      Memberships map[string]MeshMembership
      Metrics     MeshMetrics
      Issues      []Issue
}