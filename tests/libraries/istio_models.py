# Generated from the Istio 1.22.0 AuthorizationPolicy CRD schema.
# Regenerate: scripts/gen-istio-models.sh (bump ISTIO_VERSION there + in Dockerfile.tests)
# Do not edit manually.

from __future__ import annotations

from enum import Enum

from pydantic import BaseModel, Field


class Action(Enum):
    ALLOW = 'ALLOW'
    DENY = 'DENY'
    AUDIT = 'AUDIT'
    CUSTOM = 'CUSTOM'


class Provider(BaseModel):
    name: str | None = Field(
        None, description='Specifies the name of the extension provider.'
    )


class Source(BaseModel):
    ipBlocks: list[str] | None = Field(None, description='Optional.')
    namespaces: list[str] | None = Field(None, description='Optional.')
    notIpBlocks: list[str] | None = Field(None, description='Optional.')
    notNamespaces: list[str] | None = Field(None, description='Optional.')
    notPrincipals: list[str] | None = Field(None, description='Optional.')
    notRemoteIpBlocks: list[str] | None = Field(None, description='Optional.')
    notRequestPrincipals: list[str] | None = Field(None, description='Optional.')
    principals: list[str] | None = Field(None, description='Optional.')
    remoteIpBlocks: list[str] | None = Field(None, description='Optional.')
    requestPrincipals: list[str] | None = Field(None, description='Optional.')


class FromItem(BaseModel):
    source: Source | None = Field(
        None, description='Source specifies the source of a request.'
    )


class Operation(BaseModel):
    hosts: list[str] | None = Field(None, description='Optional.')
    methods: list[str] | None = Field(None, description='Optional.')
    notHosts: list[str] | None = Field(None, description='Optional.')
    notMethods: list[str] | None = Field(None, description='Optional.')
    notPaths: list[str] | None = Field(None, description='Optional.')
    notPorts: list[str] | None = Field(None, description='Optional.')
    paths: list[str] | None = Field(None, description='Optional.')
    ports: list[str] | None = Field(None, description='Optional.')


class ToItem(BaseModel):
    operation: Operation | None = Field(
        None, description='Operation specifies the operation of a request.'
    )


class WhenItem(BaseModel):
    key: str = Field(..., description='The name of an Istio attribute.')
    notValues: list[str] | None = Field(None, description='Optional.')
    values: list[str] | None = Field(None, description='Optional.')


class Rule(BaseModel):
    from_: list[FromItem] | None = Field(None, alias='from', description='Optional.')
    to: list[ToItem] | None = Field(None, description='Optional.')
    when: list[WhenItem] | None = Field(None, description='Optional.')


class Selector(BaseModel):
    matchLabels: dict[str, str] | None = Field(
        None,
        description='One or more labels that indicate a specific set of pods/VMs on which a policy should be applied.',
    )


class TargetRef(BaseModel):
    group: str | None = Field(
        None, description='group is the group of the target resource.'
    )
    kind: str | None = Field(None, description='kind is kind of the target resource.')
    name: str | None = Field(
        None, description='name is the name of the target resource.'
    )
    namespace: str | None = Field(
        None, description='namespace is the namespace of the referent.'
    )


class AuthorizationPolicySpec(BaseModel):
    action: Action | None = Field(
        None, description='Optional.\n\nValid Options: ALLOW, DENY, AUDIT, CUSTOM'
    )
    provider: Provider | None = Field(
        None, description='Specifies detailed configuration of the CUSTOM action.'
    )
    rules: list[Rule] | None = Field(None, description='Optional.')
    selector: Selector | None = Field(None, description='Optional.')
    targetRef: TargetRef | None = None
    targetRefs: list[TargetRef] | None = Field(None, description='Optional.')
