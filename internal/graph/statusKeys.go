package graph

// StatusKey mirrors the UI's StatusKey union type — computed by the backend,
// rendered by the UI as icon badges on workload nodes.
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