# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Core configuration loading, decryption, and environment management for modenv."""

from __future__ import annotations

import base64
import copy
import dataclasses
import json
import os
import re
import tomllib
import urllib.error
import urllib.request
from collections.abc import Callable
from typing import TypeVar, overload

T = TypeVar("T")


@dataclasses.dataclass
class SecretsStoreConfig:
    """Configuration for secrets resolution store."""

    type: str = "simple"
    google_cloud_project: str = ""
    google_cloud_region: str = ""
    key_location: str = ""


# Global resolver hook for custom cloud secret resolution
_cloud_secret_resolver: Callable[[str, SecretsStoreConfig], str] | None = None


def set_cloud_secret_resolver(
    resolver: Callable[[str, SecretsStoreConfig], str] | None,
) -> None:
    """Set a custom hook for resolving cloud secret URIs."""
    global _cloud_secret_resolver
    _cloud_secret_resolver = resolver


class EnvManager:
    """Tracks changes to environment variables and allows reverting them to their original states."""

    def __init__(self) -> None:
        self._original: dict[str, str | None] = {}

    def set(self, key: str, value: str) -> None:
        """Set an environment variable and record original value if not tracked."""
        if key not in self._original:
            self._original[key] = os.environ.get(key)
        os.environ[key] = value

    def get(self, key: str, default: str | None = None) -> str | None:
        """Retrieve the current value of the environment variable named by key."""
        return os.environ.get(key, default)

    def lookup(self, key: str) -> tuple[str, bool]:
        """Retrieve value and report whether environment variable was present."""
        if key in os.environ:
            return os.environ[key], True
        return "", False

    def unset(self, key: str) -> None:
        """Unset an environment variable and record original value if not tracked."""
        if key not in self._original:
            self._original[key] = os.environ.get(key)
        if key in os.environ:
            del os.environ[key]

    def restore(self) -> None:
        """Revert all changes made via this EnvManager to their original values."""
        for key, original_val in self._original.items():
            if original_val is None:
                if key in os.environ:
                    del os.environ[key]
            else:
                os.environ[key] = original_val
        self._original.clear()


def encrypt_secret(plain_text: str) -> str:
    """Encrypt a plain text string using XOR and return hex encoded string prefixed with 'simple://'."""
    key_bytes = _get_secret_key().encode("utf-8")
    input_bytes = plain_text.encode("utf-8")
    output = bytes(b ^ key_bytes[i % len(key_bytes)] for i, b in enumerate(input_bytes))
    return f"simple://{output.hex()}"


def encrypt_legacy_secret(plain_text: str) -> str:
    """Encrypt a plain text string using XOR and return hex encoded string prefixed with 'xor:'."""
    key_bytes = _get_secret_key().encode("utf-8")
    input_bytes = plain_text.encode("utf-8")
    output = bytes(b ^ key_bytes[i % len(key_bytes)] for i, b in enumerate(input_bytes))
    return f"xor:{output.hex()}"


def decrypt_secret(encoded_text: str) -> str:
    """Decrypt an encoded string starting with 'simple://' or 'xor:' and return the plain text."""
    if encoded_text.startswith("simple://"):
        hex_str = encoded_text[9:]
    elif encoded_text.startswith("xor:"):
        hex_str = encoded_text[4:]
    else:
        raise ValueError("invalid secret format: missing 'simple://' or 'xor:' prefix")
    try:
        data = bytes.fromhex(hex_str)
    except ValueError as exc:
        raise ValueError(f"failed to hex decode secret: {exc}") from exc
    key_bytes = _get_secret_key().encode("utf-8")
    output = bytes(b ^ key_bytes[i % len(key_bytes)] for i, b in enumerate(data))
    return output.decode("utf-8")


def _parse_asn1_length(data: bytes, offset: int) -> tuple[int, int]:
    """Parse ASN.1 DER length field."""
    b = data[offset]
    offset += 1
    if b < 0x80:
        return b, offset
    n = b & 0x7F
    if offset + n > len(data):
        raise ValueError("ASN.1 length exceeds buffer bounds")
    length = int.from_bytes(data[offset : offset + n], "big")
    return length, offset + n


def _parse_asn1_integer(data: bytes, offset: int) -> tuple[int, int]:
    """Parse ASN.1 DER INTEGER element."""
    if offset >= len(data) or data[offset] != 0x02:
        raise ValueError("Expected ASN.1 INTEGER (tag 0x02)")
    offset += 1
    length, offset = _parse_asn1_length(data, offset)
    val = int.from_bytes(data[offset : offset + length], "big")
    return val, offset + length


def _parse_private_key_pem(pem_str: str) -> tuple[int, int]:
    """Parse RSA private key PEM (PKCS#1 or PKCS#8) returning (modulus, private_exponent)."""
    lines = [
        line.strip()
        for line in pem_str.splitlines()
        if line.strip() and not line.startswith("-----")
    ]
    if not lines:
        raise ValueError("failed to decode PEM block for private key")
    try:
        der = base64.b64decode("".join(lines))
    except Exception as exc:
        raise ValueError(f"failed to decode base64 for private key: {exc}") from exc

    offset = 0
    if offset >= len(der) or der[offset] != 0x30:
        raise ValueError("Expected ASN.1 SEQUENCE")
    offset += 1
    _, offset = _parse_asn1_length(der, offset)
    _version, offset = _parse_asn1_integer(der, offset)

    # Check if this is PKCS#8 (has an AlgorithmIdentifier sequence next)
    if offset < len(der) and der[offset] == 0x30:
        seq_len, offset = _parse_asn1_length(der, offset + 1)
        offset += seq_len
        if offset >= len(der) or der[offset] != 0x04:
            raise ValueError("Expected OCTET STRING in PKCS#8 private key")
        oct_len, offset = _parse_asn1_length(der, offset + 1)
        der = der[offset : offset + oct_len]
        offset = 0
        if offset >= len(der) or der[offset] != 0x30:
            raise ValueError("Expected inner SEQUENCE in PKCS#8")
        offset += 1
        _, offset = _parse_asn1_length(der, offset)
        _version, offset = _parse_asn1_integer(der, offset)

    n, offset = _parse_asn1_integer(der, offset)
    _e, offset = _parse_asn1_integer(der, offset)
    d, offset = _parse_asn1_integer(der, offset)
    return n, d


def _parse_public_key_pem(pem_str: str) -> tuple[int, int]:
    """Parse RSA public key PEM (SPKI or PKCS#1) returning (modulus, public_exponent)."""
    lines = [
        line.strip()
        for line in pem_str.splitlines()
        if line.strip() and not line.startswith("-----")
    ]
    if not lines:
        raise ValueError("failed to parse PEM block containing public key")
    try:
        der = base64.b64decode("".join(lines))
    except Exception as exc:
        raise ValueError(f"failed to decode base64 for public key: {exc}") from exc

    offset = 0
    if offset >= len(der) or der[offset] != 0x30:
        raise ValueError("Expected ASN.1 SEQUENCE")
    offset += 1
    _, offset = _parse_asn1_length(der, offset)

    # Check if this is SubjectPublicKeyInfo (AlgorithmIdentifier sequence first)
    if offset < len(der) and der[offset] == 0x30:
        alg_len, offset = _parse_asn1_length(der, offset + 1)
        offset += alg_len
        if offset >= len(der) or der[offset] != 0x03:
            raise ValueError("Expected BIT STRING in SPKI public key")
        _bit_len, offset = _parse_asn1_length(der, offset + 1)
        # Skip unused bits byte
        offset += 1
        der = der[offset:]
        offset = 0
        if offset >= len(der) or der[offset] != 0x30:
            raise ValueError("Expected inner SEQUENCE in RSA public key")
        offset += 1
        _, offset = _parse_asn1_length(der, offset)

    n, offset = _parse_asn1_integer(der, offset)
    e, offset = _parse_asn1_integer(der, offset)
    return n, e


def encrypt_pks_secret(plain_text: str, pub_key_pem: str) -> str:
    """Encrypt a plaintext using RSA PKCS#1 v1.5 padding and an RSA public key PEM."""
    n, e = _parse_public_key_pem(pub_key_pem)
    data = plain_text.encode("utf-8")
    k = (n.bit_length() + 7) // 8
    if len(data) > k - 11:
        raise ValueError("message too long for RSA key size")

    pad_len = k - len(data) - 3
    ps = bytearray()
    while len(ps) < pad_len:
        b = os.urandom(pad_len - len(ps))
        ps.extend(x for x in b if x != 0)

    em = b"\x00\x02" + bytes(ps) + b"\x00" + data
    m = int.from_bytes(em, "big")
    c = pow(m, e, n)
    c_bytes = c.to_bytes(k, "big")
    return "pks://" + base64.b64encode(c_bytes).decode("ascii")


def decrypt_pks_secret(encoded_text: str, cfg: SecretsStoreConfig | None = None) -> str:
    """Decrypt a 'pks://' prefixed base64 RSA encrypted secret."""
    if not encoded_text.startswith("pks://"):
        raise ValueError("invalid secret format: missing 'pks://' prefix")
    raw_b64 = encoded_text[6:]

    # Resolve private key PEM
    pem_data = os.environ.get("MODENV_PRIVATE_KEY")
    if not pem_data:
        key_path = os.environ.get("MODENV_KEY_LOCATION")
        if not key_path and cfg and cfg.key_location:
            key_path = cfg.key_location

        if not key_path:
            raise ValueError(
                "cannot decrypt PKS secret: no private key found in key_location, "
                "MODENV_KEY_LOCATION, or MODENV_PRIVATE_KEY"
            )

        target_path = key_path
        if not os.path.isabs(target_path) and not os.path.exists(target_path):
            prefix = os.environ.get("MODENV_PREFIX")
            if prefix:
                target_path = os.path.join(prefix, target_path)

        if not os.path.isfile(target_path):
            raise FileNotFoundError(f"could not read private key file at {target_path}")

        with open(target_path, "r", encoding="utf-8") as f:
            pem_data = f.read()

    n, d = _parse_private_key_pem(pem_data)
    try:
        cipher_bytes = base64.b64decode(raw_b64)
    except Exception as exc:
        raise ValueError(f"failed to decode base64 ciphertext: {exc}") from exc

    c = int.from_bytes(cipher_bytes, "big")
    m = pow(c, d, n)
    k = (n.bit_length() + 7) // 8
    em = m.to_bytes(k, "big")

    if em[0] != 0x00 or em[1] != 0x02:
        raise ValueError("decryption error: invalid PKCS#1 v1.5 padding block type")

    try:
        sep = em.index(b"\x00", 2)
    except ValueError as exc:
        raise ValueError(
            "decryption error: missing 0x00 separator in PKCS#1 v1.5 padding"
        ) from exc

    if sep < 10:
        raise ValueError("decryption error: padding too short")

    return em[sep + 1 :].decode("utf-8")


def resolve_cloud_secret(uri: str, cfg: SecretsStoreConfig | None = None) -> str:
    """Resolve a secret from Google Cloud Secret Manager URI."""
    if not uri.startswith("cloud://"):
        raise ValueError(f"invalid cloud secret URI: {uri}")

    trimmed = uri[8:]
    project = ""
    secret = ""
    version = "latest"

    match = re.match(
        r"^projects/([^/]+)/secrets/([^/]+)(?:/versions/([^/]+))?$", trimmed
    )
    if match:
        project = match.group(1)
        secret = match.group(2)
        if match.group(3):
            version = match.group(3)
    else:
        if ":" in trimmed:
            secret, version = trimmed.split(":", 1)
        else:
            secret = trimmed
        if cfg and cfg.google_cloud_project:
            project = cfg.google_cloud_project
        if not project:
            project = (
                os.environ.get("GOOGLE_CLOUD_PROJECT")
                or os.environ.get("GCP_PROJECT")
                or os.environ.get("MODENV_GCP_PROJECT")
                or ""
            )

    if not secret:
        raise ValueError(f"cloud secret URI missing secret name: {uri}")
    if (
        any(c in secret for c in "/ \r\n")
        or any(c in project for c in "/ \r\n")
        or any(c in version for c in "/ \r\n")
    ):
        raise ValueError(
            f"invalid secret, project, or version identifier in cloud secret URI: {uri}"
        )

    if not project:
        raise ValueError(
            f"cannot resolve cloud secret {uri}: Google Cloud Project ID is not configured"
        )

    # Custom resolver hook
    if _cloud_secret_resolver is not None:
        return _cloud_secret_resolver(uri, cfg or SecretsStoreConfig())

    # Environment override for hermetic offline testing
    override_key = f"MODENV_CLOUD_SECRET_{secret.upper().replace('-', '_')}"
    if override_val := os.environ.get(override_key):
        return override_val

    # Resolve GCP Access Token
    token = (
        os.environ.get("GOOGLE_OAUTH_ACCESS_TOKEN")
        or os.environ.get("GCP_ACCESS_TOKEN")
        or ""
    )
    if not token:
        try:
            meta_req = urllib.request.Request(
                "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token",
                headers={"Metadata-Flavor": "Google"},
            )
            with urllib.request.urlopen(meta_req, timeout=1.5) as resp:
                if resp.status == 200:
                    meta_token = json.loads(resp.read().decode("utf-8"))
                    token = meta_token.get("access_token", "")
        except (urllib.error.URLError, TimeoutError, OSError, json.JSONDecodeError):
            token = ""

    if not token:
        clean_secret = secret.upper().replace("-", "_")
        raise RuntimeError(
            f"failed to resolve cloud secret '{secret}': no Google Cloud credentials or "
            f"MODENV_CLOUD_SECRET_{clean_secret} found"
        )
    if "\r" in token or "\n" in token:
        raise ValueError("invalid OAuth token: contains newline characters")

    base_url = (
        os.environ.get("MODENV_GCP_ENDPOINT") or "https://secretmanager.googleapis.com"
    )
    api_url = (
        f"{base_url}/v1/projects/{project}/secrets/{secret}/versions/{version}:access"
    )

    req = urllib.request.Request(api_url, headers={"Authorization": f"Bearer {token}"})
    try:
        with urllib.request.urlopen(req, timeout=5.0) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            b64_payload = data.get("payload", {}).get("data", "")
            return base64.b64decode(b64_payload).decode("utf-8")
    except Exception as exc:
        raise RuntimeError(f"failed to fetch cloud secret '{secret}': {exc}") from exc


def _get_secret_key() -> str:
    """Resolve encryption key from environment variable with fallback."""
    return os.environ.get("MODENV_KEY") or "modenv-default-key"


def _resolve_path(filename: str) -> str:
    """Resolve target path joined with MODENV_PREFIX if configured."""
    prefix = os.environ.get("MODENV_PREFIX")
    if not prefix:
        return filename
    return os.path.join(prefix, filename)


def _load_file(path: str) -> dict[str, object]:
    """Read a TOML file and parse into a dictionary."""
    with open(path, "rb") as f:
        return tomllib.load(f)


def _deep_merge(dst: dict[str, object], src: dict[str, object]) -> dict[str, object]:
    """Recursively merge src dictionary into dst dictionary."""
    for key, value in src.items():
        if key in dst and isinstance(dst[key], dict) and isinstance(value, dict):
            _deep_merge(dst[key], value)  # type: ignore[arg-type]
        else:
            dst[key] = value
    return dst


def _decrypt_config_in_place(obj: object, cfg: SecretsStoreConfig) -> None:
    """Recursively decrypt values starting with simple://, xor:, pks://, or cloud://."""
    if isinstance(obj, dict):
        for key, value in list(obj.items()):
            if isinstance(value, str):
                if value.startswith(("simple://", "xor:")):
                    obj[key] = decrypt_secret(value)
                elif value.startswith("pks://"):
                    obj[key] = decrypt_pks_secret(value, cfg)
                elif value.startswith("cloud://"):
                    obj[key] = resolve_cloud_secret(value, cfg)
            elif isinstance(value, (dict, list)):
                _decrypt_config_in_place(value, cfg)
    elif isinstance(obj, list):
        for index, item in enumerate(obj):
            if isinstance(item, str):
                if item.startswith(("simple://", "xor:")):
                    obj[index] = decrypt_secret(item)
                elif item.startswith("pks://"):
                    obj[index] = decrypt_pks_secret(item, cfg)
                elif item.startswith("cloud://"):
                    obj[index] = resolve_cloud_secret(item, cfg)
            elif isinstance(item, (dict, list)):
                _decrypt_config_in_place(item, cfg)


def _bind_to_object(target: object, data: dict[str, object]) -> object:
    """Bind dictionary data to fields of a class instance or dataclass."""
    if dataclasses.is_dataclass(target) and not isinstance(target, type):
        for f in dataclasses.fields(target):
            if f.name in data:
                val = data[f.name]
                current = getattr(target, f.name)
                if (
                    dataclasses.is_dataclass(current) or hasattr(current, "__dict__")
                ) and isinstance(val, dict):
                    _bind_to_object(current, val)
                else:
                    setattr(target, f.name, val)
        return target

    if hasattr(target, "__dict__"):
        for k, v in data.items():
            if hasattr(target, k):
                attr = getattr(target, k)
                if (
                    dataclasses.is_dataclass(attr) or hasattr(attr, "__dict__")
                ) and isinstance(v, dict):
                    _bind_to_object(attr, v)
                else:
                    setattr(target, k, v)
            else:
                setattr(target, k, v)
        return target

    return data


@overload
def load(target: None = None) -> dict[str, object]: ...


@overload
def load(target: T) -> T: ...


def load(target: T | None = None) -> T | dict[str, object]:
    """Load and parse hierarchical TOML environment configurations.

    Precedence:
    1. .env.toml (required)
    2. .env.${MODENV_RUNTIME}.toml (optional runtime override)
    3. .env.local.toml (optional local override, loaded last)

    Paths are resolved relative to MODENV_PREFIX if configured.
    If target is None, returns a defensive copy of the merged dictionary.
    If target is provided, binds the merged config to target and returns a defensive copy.
    """
    base_file = _resolve_path(".env.toml")
    if not os.path.isfile(base_file):
        raise FileNotFoundError(
            f"failed to load base config {base_file}: file not found"
        )

    merged = _load_file(base_file)

    runtime = os.environ.get("MODENV_RUNTIME")
    if runtime:
        runtime_file = _resolve_path(f".env.{runtime}.toml")
        if os.path.isfile(runtime_file):
            runtime_map = _load_file(runtime_file)
            _deep_merge(merged, runtime_map)

    local_file = _resolve_path(".env.local.toml")
    if os.path.isfile(local_file):
        local_map = _load_file(local_file)
        _deep_merge(merged, local_map)

    # Extract secrets_store configuration
    cfg = SecretsStoreConfig()
    if "secrets_store" in merged and isinstance(merged["secrets_store"], dict):
        store_dict = merged["secrets_store"]
        cfg = SecretsStoreConfig(
            type=str(store_dict.get("type", "simple")),
            google_cloud_project=str(store_dict.get("google_cloud_project", "")),
            google_cloud_region=str(store_dict.get("google_cloud_region", "")),
            key_location=str(store_dict.get("key_location", "")),
        )

    _decrypt_config_in_place(merged, cfg)

    if target is None:
        return copy.deepcopy(merged)

    if isinstance(target, dict):
        target.clear()
        target.update(copy.deepcopy(merged))
        return copy.deepcopy(target)

    _bind_to_object(target, merged)
    return copy.deepcopy(target)
