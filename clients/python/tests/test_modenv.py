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

"""Unit tests for modenv Python package."""

from __future__ import annotations

import dataclasses
import os
import unittest
from pathlib import Path

from modenv import (
    EnvManager,
    SecretsStoreConfig,
    decrypt_pks_secret,
    decrypt_secret,
    encrypt_legacy_secret,
    encrypt_pks_secret,
    encrypt_secret,
    load,
    resolve_cloud_secret,
    set_cloud_secret_resolver,
)


@dataclasses.dataclass
class DBConfig:
    host: str = ""
    user: str = ""
    password: str = ""


@dataclasses.dataclass
class AppConfig:
    app_name: str = ""
    port: int = 0
    features: list[str] = dataclasses.field(default_factory=list)
    database: DBConfig = dataclasses.field(default_factory=DBConfig)


def _get_test_configs_dir() -> str:
    current = Path(__file__).resolve().parent
    # Check tests/configs
    direct = current / "configs"
    if direct.is_dir():
        return str(direct)
    # Check ../test/configs or workspace root
    for parent in [current.parent, current.parent.parent, current.parent.parent.parent]:
        candidate = parent / "test" / "configs"
        if candidate.is_dir():
            return str(candidate)
        candidate2 = parent / "configs"
        if candidate2.is_dir() and (candidate2 / "default").is_dir():
            return str(candidate2)
    return str(direct)


class TestModenv(unittest.TestCase):
    def setUp(self) -> None:
        self._env_backup = dict(os.environ)

    def tearDown(self) -> None:
        os.environ.clear()
        os.environ.update(self._env_backup)
        set_cloud_secret_resolver(None)

    def test_secret_encryption_decryption_roundtrip(self) -> None:
        plain = "my-super-secret-password-123!"

        # Default key (simple://)
        encoded = encrypt_secret(plain)
        self.assertTrue(encoded.startswith("simple://"))
        decoded = decrypt_secret(encoded)
        self.assertEqual(decoded, plain)

        # Legacy key (xor:)
        legacy_encoded = encrypt_legacy_secret(plain)
        self.assertTrue(legacy_encoded.startswith("xor:"))
        legacy_decoded = decrypt_secret(legacy_encoded)
        self.assertEqual(legacy_decoded, plain)

        # Custom key
        os.environ["MODENV_KEY"] = "custom-super-long-key-spec"
        encoded_custom = encrypt_secret(plain)
        self.assertNotEqual(encoded, encoded_custom)
        decoded_custom = decrypt_secret(encoded_custom)
        self.assertEqual(decoded_custom, plain)

    def test_secret_decryption_errors(self) -> None:
        with self.assertRaises(ValueError):
            decrypt_secret("invalid-prefix:1234")
        with self.assertRaises(ValueError):
            decrypt_secret("simple://not-valid-hex!")
        with self.assertRaises(ValueError):
            decrypt_secret("xor://not-valid-hex!")

    def test_load_with_encrypted_secrets(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "secrets")
        os.environ["MODENV_PREFIX"] = configs_dir

        cfg = load()
        self.assertIsInstance(cfg, dict)
        self.assertEqual(cfg.get("app_name"), "secret-app")
        db = cfg.get("database")
        self.assertIsInstance(db, dict)
        self.assertEqual(db.get("password"), "local_db_password")  # type: ignore[union-attr]

    def test_load_with_prefix(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "prefix-test")
        os.environ["MODENV_PREFIX"] = configs_dir

        cfg = load()
        self.assertEqual(cfg.get("app_name"), "prefixed-app")
        self.assertEqual(cfg.get("port"), 2000)

    def test_load_map_default(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "default")
        os.environ["MODENV_PREFIX"] = configs_dir

        cfg = load()
        self.assertEqual(cfg.get("app_name"), "modenv-default")
        self.assertEqual(cfg.get("port"), 8080)
        db = cfg.get("database")
        self.assertIsInstance(db, dict)
        self.assertEqual(db.get("host"), "localhost")  # type: ignore[union-attr]

    def test_load_struct(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "struct")
        os.environ["MODENV_PREFIX"] = configs_dir

        cfg = AppConfig()
        clone = load(cfg)

        self.assertEqual(cfg.app_name, "modenv-struct")
        self.assertEqual(cfg.port, 9000)
        self.assertEqual(cfg.features, ["web"])
        self.assertEqual(cfg.database.host, "db")

        self.assertEqual(clone.app_name, "modenv-struct")
        self.assertEqual(clone.port, 9000)

    def test_load_runtime_override(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "runtime")
        os.environ["MODENV_PREFIX"] = configs_dir
        os.environ["MODENV_RUNTIME"] = "production"

        cfg = load()
        self.assertEqual(cfg.get("app_name"), "base")
        self.assertEqual(cfg.get("port"), 9999)
        db = cfg.get("database")
        self.assertIsInstance(db, dict)
        self.assertEqual(db.get("host"), "prod-db")  # type: ignore[union-attr]

    def test_load_local_override_last(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "local-override")
        os.environ["MODENV_PREFIX"] = configs_dir
        os.environ["MODENV_RUNTIME"] = "production"

        cfg = load()
        self.assertEqual(cfg.get("app_name"), "prod")
        self.assertEqual(cfg.get("port"), 7777)
        db = cfg.get("database")
        self.assertIsInstance(db, dict)
        self.assertEqual(db.get("host"), "local-db")  # type: ignore[union-attr]

    def test_load_defensive_copy(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "defensive")
        os.environ["MODENV_PREFIX"] = configs_dir

        target = AppConfig()
        clone = load(target)

        self.assertEqual(target.app_name, "defensive")
        self.assertEqual(clone.app_name, "defensive")

        clone.app_name = "mutated"
        clone.features[0] = "mutated-feature"
        clone.database.host = "mutated-host"

        self.assertEqual(target.app_name, "defensive")
        self.assertEqual(target.features[0], "original")
        self.assertEqual(target.database.host, "original-host")

    def test_load_missing_base_file_raises_error(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "missing")
        os.environ["MODENV_PREFIX"] = configs_dir

        with self.assertRaises(FileNotFoundError):
            load()

    def test_env_manager(self) -> None:
        manager = EnvManager()
        pre_existing_key = "TEST_PRE_EXISTING"
        pre_existing_val = "original_value"
        os.environ[pre_existing_key] = pre_existing_val

        # Set new
        manager.set("TEST_NEW_KEY", "new_val")
        val, exists = manager.lookup("TEST_NEW_KEY")
        self.assertTrue(exists)
        self.assertEqual(val, "new_val")

        # Modify existing
        manager.set(pre_existing_key, "modified_val")
        val, exists = manager.lookup(pre_existing_key)
        self.assertTrue(exists)
        self.assertEqual(val, "modified_val")

        # Unset
        manager.unset(pre_existing_key)
        val, exists = manager.lookup(pre_existing_key)
        self.assertFalse(exists)

        # Restore
        manager.restore()
        self.assertNotIn("TEST_NEW_KEY", os.environ)
        self.assertIn(pre_existing_key, os.environ)
        self.assertEqual(os.environ[pre_existing_key], pre_existing_val)

        # Extra coverage for get and unsetting untracked variable
        self.assertIsNone(manager.get("COMPLETELY_UNKNOWN_VAR"))
        self.assertEqual(manager.get("COMPLETELY_UNKNOWN_VAR", "fallback"), "fallback")
        manager.unset("COMPLETELY_UNKNOWN_VAR")
        manager.restore()

    def test_invalid_secrets(self) -> None:
        with self.assertRaises(ValueError):
            decrypt_secret("not-prefixed")
        with self.assertRaises(ValueError):
            decrypt_secret("xor:zz-not-hex")

    def test_load_with_list_secrets_and_nested_structures(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as tmp_dir:
            secret1 = encrypt_secret("sec-1")
            secret2 = encrypt_secret("sec-2")
            content = (
                f'app_name = "test"\n'
                f'secret_list = ["{secret1}", "plain"]\n'
                f'nested_list = [["{secret2}"]]\n'
                f'nested_dict_list = [{{ key = "{secret1}" }}]\n'
            )
            with open(os.path.join(tmp_dir, ".env.toml"), "w") as f:
                f.write(content)

            os.environ["MODENV_PREFIX"] = tmp_dir
            res = load()
            self.assertEqual(res["secret_list"], ["sec-1", "plain"])
            self.assertEqual(res["nested_list"], [["sec-2"]])
            self.assertEqual(res["nested_dict_list"], [{"key": "sec-1"}])

    def test_load_with_plain_class(self) -> None:
        import tempfile

        class PlainConfig:
            def __init__(self) -> None:
                self.app_name = ""
                self.port = 0

        with tempfile.TemporaryDirectory() as tmp_dir:
            with open(os.path.join(tmp_dir, ".env.toml"), "w") as f:
                f.write('app_name = "plain-app"\nport = 5050\n')

            os.environ["MODENV_PREFIX"] = tmp_dir
            cfg = PlainConfig()
            res = load(cfg)
            self.assertEqual(res.app_name, "plain-app")
            self.assertEqual(res.port, 5050)

    def test_load_with_primitive_target(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as tmp_dir:
            with open(os.path.join(tmp_dir, ".env.toml"), "w") as f:
                f.write('app_name = "prim"\n')

            os.environ["MODENV_PREFIX"] = tmp_dir
            res = load(123)  # type: ignore[call-overload]
            self.assertEqual(res, 123)

    def test_load_without_prefix(self) -> None:
        import tempfile

        with tempfile.TemporaryDirectory() as tmp_dir:
            with open(os.path.join(tmp_dir, ".env.toml"), "w") as f:
                f.write('app_name = "no-prefix"\n')

            orig_cwd = os.getcwd()
            os.chdir(tmp_dir)
            try:
                if "MODENV_PREFIX" in os.environ:
                    del os.environ["MODENV_PREFIX"]
                res = load()
                self.assertEqual(res["app_name"], "no-prefix")
            finally:
                os.chdir(orig_cwd)

    def test_pks_encryption_decryption(self) -> None:
        configs_dir = _get_test_configs_dir()
        pub_path = os.path.join(configs_dir, "test_rsa_public.pem")
        priv_path = os.path.join(configs_dir, "test_rsa_private.pem")

        with open(pub_path, "r", encoding="utf-8") as f:
            pub_pem = f.read()
        with open(priv_path, "r", encoding="utf-8") as f:
            priv_pem = f.read()

        secret = "super-secret-rsa-payload"
        encrypted = encrypt_pks_secret(secret, pub_pem)
        self.assertTrue(encrypted.startswith("pks://"))

        # Decrypt using MODENV_PRIVATE_KEY
        os.environ["MODENV_PRIVATE_KEY"] = priv_pem
        decrypted = decrypt_pks_secret(encrypted)
        self.assertEqual(decrypted, secret)
        del os.environ["MODENV_PRIVATE_KEY"]

        # Decrypt using MODENV_KEY_LOCATION
        os.environ["MODENV_KEY_LOCATION"] = priv_path
        decrypted = decrypt_pks_secret(encrypted)
        self.assertEqual(decrypted, secret)
        del os.environ["MODENV_KEY_LOCATION"]

        # Decrypt using SecretsStoreConfig
        cfg = SecretsStoreConfig(type="pks", key_location=priv_path)
        decrypted = decrypt_pks_secret(encrypted, cfg)
        self.assertEqual(decrypted, secret)

    def test_pks_errors(self) -> None:
        configs_dir = _get_test_configs_dir()
        pub_path = os.path.join(configs_dir, "test_rsa_public.pem")
        with open(pub_path, "r", encoding="utf-8") as f:
            pub_pem = f.read()

        # Invalid prefix
        with self.assertRaises(ValueError):
            decrypt_pks_secret("xor:1234")

        # Missing key
        with self.assertRaises(ValueError):
            decrypt_pks_secret("pks://aaaa", SecretsStoreConfig(type="pks"))

        # Nonexistent key file
        with self.assertRaises(FileNotFoundError):
            decrypt_pks_secret(
                "pks://aaaa",
                SecretsStoreConfig(type="pks", key_location="/no/such/file.pem"),
            )

        # Bad PEM
        os.environ["MODENV_PRIVATE_KEY"] = "not a valid pem"
        with self.assertRaises(ValueError):
            decrypt_pks_secret("pks://aaaa")
        del os.environ["MODENV_PRIVATE_KEY"]

        # Message too long
        with self.assertRaises(ValueError):
            encrypt_pks_secret("x" * 500, pub_pem)

    def test_cloud_secret_with_env_override(self) -> None:
        os.environ["MODENV_CLOUD_SECRET_DB_PASS"] = "env-override-password"
        cfg = SecretsStoreConfig(type="cloud", google_cloud_project="my-proj")

        val = resolve_cloud_secret("cloud://db-pass", cfg)
        self.assertEqual(val, "env-override-password")

        # Test full URI
        os.environ["MODENV_CLOUD_SECRET_FULL_SECRET"] = "full-secret-val"
        val2 = resolve_cloud_secret(
            "cloud://projects/my-proj/secrets/full-secret/versions/1", cfg
        )
        self.assertEqual(val2, "full-secret-val")

    def test_cloud_secret_with_custom_resolver(self) -> None:
        def custom_resolver(uri: str, cfg: SecretsStoreConfig) -> str:
            return f"custom-{uri}"

        set_cloud_secret_resolver(custom_resolver)
        val = resolve_cloud_secret(
            "cloud://some-secret",
            SecretsStoreConfig(type="cloud", google_cloud_project="proj"),
        )
        self.assertEqual(val, "custom-cloud://some-secret")
        set_cloud_secret_resolver(None)

    def test_cloud_secret_with_mock_endpoint(self) -> None:
        import base64
        import json
        import threading
        from http.server import BaseHTTPRequestHandler, HTTPServer

        class MockGCPHandler(BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                if "db-pass" in self.path:
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.end_headers()
                    payload = {
                        "payload": {
                            "data": base64.b64encode(b"server-secret-42").decode(
                                "ascii"
                            )
                        }
                    }
                    self.wfile.write(json.dumps(payload).encode("utf-8"))
                else:
                    self.send_response(404)
                    self.end_headers()

            def log_message(self, format: str, *args: object) -> None:
                pass

        server = HTTPServer(("127.0.0.1", 0), MockGCPHandler)
        server_thread = threading.Thread(target=server.serve_forever, daemon=True)
        server_thread.start()

        try:
            port = server.server_port
            os.environ["MODENV_GCP_ENDPOINT"] = f"http://127.0.0.1:{port}"
            os.environ["GOOGLE_OAUTH_ACCESS_TOKEN"] = "mock-token-abc"
            cfg = SecretsStoreConfig(type="cloud", google_cloud_project="test-proj")

            val = resolve_cloud_secret("cloud://db-pass", cfg)
            self.assertEqual(val, "server-secret-42")

            with self.assertRaises(RuntimeError):
                resolve_cloud_secret("cloud://not-found", cfg)
        finally:
            server.shutdown()
            server.server_close()

    def test_cloud_secret_errors(self) -> None:
        # Invalid URI
        with self.assertRaises(ValueError):
            resolve_cloud_secret("invalid://uri")

        # Missing project
        if "GOOGLE_CLOUD_PROJECT" in os.environ:
            del os.environ["GOOGLE_CLOUD_PROJECT"]
        if "GCP_PROJECT" in os.environ:
            del os.environ["GCP_PROJECT"]
        with self.assertRaises(ValueError):
            resolve_cloud_secret("cloud://secret", SecretsStoreConfig())

        # No credentials or override
        cfg = SecretsStoreConfig(type="cloud", google_cloud_project="proj")
        with self.assertRaises(RuntimeError):
            resolve_cloud_secret("cloud://unconfigured-secret", cfg)

    def test_load_smart_secrets_directory(self) -> None:
        configs_dir = _get_test_configs_dir()
        smart_dir = os.path.join(configs_dir, "smart-secrets")
        priv_path = os.path.join(configs_dir, "test_rsa_private.pem")

        os.environ["MODENV_PREFIX"] = smart_dir
        os.environ["MODENV_KEY_LOCATION"] = priv_path
        os.environ["MODENV_CLOUD_SECRET_MY_DB_SECRET"] = "cloud-resolved-pass"
        os.environ["MODENV_CLOUD_SECRET_MY_FULL_SECRET"] = "full-cloud-pass"

        cfg = load()
        self.assertEqual(cfg["app_name"], "smart-secrets-app")

        db = cfg["database"]
        self.assertIsInstance(db, dict)
        self.assertEqual(db["simple_password"], "local_db_password")
        self.assertEqual(db["legacy_password"], "local_db_password")
        self.assertEqual(db["pks_password"], "pks-decrypted-secret-42")
        self.assertEqual(db["cloud_password"], "cloud-resolved-pass")
        self.assertEqual(db["cloud_full_uri"], "full-cloud-pass")


if __name__ == "__main__":
    unittest.main()
