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

"""Integration tests for modenv Python package."""

from __future__ import annotations

import dataclasses
import os
import unittest
from pathlib import Path

from modenv import EnvManager, load


@dataclasses.dataclass
class IntegrationConfig:
    integration_val: str = ""
    port: int = 0


def _get_test_configs_dir() -> str:
    current = Path(__file__).resolve().parent
    direct = current / "configs"
    if direct.is_dir():
        return str(direct)
    for parent in [current] + list(current.parents):
        candidate = parent / "test" / "configs"
        if candidate.is_dir():
            return str(candidate)
        candidate2 = parent / "configs"
        if candidate2.is_dir() and (candidate2 / "integration").is_dir():
            return str(candidate2)
    return str(direct)


class TestIntegration(unittest.TestCase):
    def test_integration_env_manager(self) -> None:
        manager = EnvManager()

        if "INTEGRATION_KEY_1" in os.environ:
            del os.environ["INTEGRATION_KEY_1"]
        if "INTEGRATION_KEY_2" in os.environ:
            del os.environ["INTEGRATION_KEY_2"]

        manager.set("INTEGRATION_KEY_1", "val1")
        manager.set("INTEGRATION_KEY_2", "val2")

        self.assertEqual(os.environ.get("INTEGRATION_KEY_1"), "val1")
        self.assertEqual(os.environ.get("INTEGRATION_KEY_2"), "val2")

        manager.restore()

        self.assertNotIn("INTEGRATION_KEY_1", os.environ)
        self.assertNotIn("INTEGRATION_KEY_2", os.environ)

    def test_integration_load(self) -> None:
        configs_dir = os.path.join(_get_test_configs_dir(), "integration")
        os.environ["MODENV_PREFIX"] = configs_dir

        cfg = IntegrationConfig()
        clone = load(cfg)

        self.assertEqual(cfg.integration_val, "base")
        self.assertEqual(cfg.port, 2000)

        self.assertEqual(clone.integration_val, "base")
        self.assertEqual(clone.port, 2000)


if __name__ == "__main__":
    unittest.main()
