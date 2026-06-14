"""Hand-written PeerAuthentication models.

Kept separate from istio_models.py (which is CRD-generated, do-not-edit). The PA
spec surface the mesh tests need is small: a selector, a top-level mTLS mode, and
optional per-port modes. Mirrors security.istio.io/v1 PeerAuthentication.
"""

from __future__ import annotations

from enum import Enum

from pydantic import BaseModel, Field

from .istio_models import Selector


class MtlsMode(Enum):
    UNSET = "UNSET"
    DISABLE = "DISABLE"
    PERMISSIVE = "PERMISSIVE"
    STRICT = "STRICT"


class Mtls(BaseModel):
    mode: MtlsMode | None = None


class PeerAuthenticationSpec(BaseModel):
    selector: Selector | None = None
    mtls: Mtls | None = None
    # port number → per-port mTLS override.
    portLevelMtls: dict[int, Mtls] | None = Field(None, description="Optional.")
