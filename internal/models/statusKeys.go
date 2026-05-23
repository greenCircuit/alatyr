package models

// rendered by the UI as icon badges on workload nodes.
// generic for k8s
type StatusKey string

const (
	StatusInternetIngress  StatusKey = "internet-ingress"
	StatusInternetEgress   StatusKey = "internet-egress"
	StatusInternetFull     StatusKey = "internet-full"
	StatusLanIngress       StatusKey = "lan-ingress"
	StatusLanEgress        StatusKey = "lan-egress"
	StatusLanFull          StatusKey = "lan-full"
	StatusApiServerEgress  StatusKey = "api-server-egress"
	StatusIsolated         StatusKey = "air-gapped"
	StatusCrossNamespace   StatusKey = "cross-namespace"
	StatusNamespaceEgress  StatusKey = "ns-egress-access"
	StatusNamespaceIngress StatusKey = "ns-ingress-access"
	StatusNamespaceFull    StatusKey = "ns-full-access"
)

// statuses that polices provide to build status keys
type PolicyStatus struct {
	EgressLocked       bool 
	IngressLocked      bool 
	InternetEgress     bool 
	InternetIngress    bool 
	LanEgress          bool 
	LanIngress         bool 
	ApiServerEgress    bool 
	CrossNS            bool 
	InnerNsEgress      bool 
	InnerNsIngress     bool 
}