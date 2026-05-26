package models

// Direction mirrors PolicyEdge.direction.
type Direction string

const (
	DirectionIngress Direction = "ingress"
	DirectionEgress  Direction = "egress"
	DirectionBoth    Direction = "both"
)
