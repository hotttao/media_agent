"""Monorepo 层级依赖检查脚本。

对标 Go archcheck，但用 Python AST 扫描 monorepo 多包之间的依赖方向。

约束：
- adapter 是被依赖方：adapter 不得 import codex/goose/claude_code
- agents 之间互不依赖：codex / goose / claude_code 不得相互 import
- agents 可以 import adapter（adapter.biz / adapter.config / adapter.infra / ...）

adapter 包内部层级依赖由 adapter/pyproject.toml 的 [tool.importlinter] 强制（lint-imports）。
本脚本只覆盖 monorepo 跨包边界，不重复检查 adapter 内部。

运行：
    python scripts/check_layers.py

退出码：
    0 = 通过
    1 = 发现违规
"""

from __future__ import annotations

import ast
import sys
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

ADAPTER_PKG = {"adapter"}  # adapter 是一个包，biz/schemas/config/infra/clients 是子包
AGENT_PKG = {"codex", "goose", "claude_code"}


@dataclass
class Violation:
    file: Path
    line: int
    importer: str  # 文件所属的包名
    imported: str  # import 的模块名
    rule: str

    def format(self) -> str:
        return (
            f"  {self.rule}\n"
            f"    file: {self.file.relative_to(ROOT)}:{self.line}\n"
            f"    import: {self.importer} -> {self.imported}\n"
        )


@dataclass
class ScanResult:
    violations: list[Violation] = field(default_factory=list)
    scanned: int = 0

    def ok(self) -> bool:
        return not self.violations


def _top_pkg(module: str) -> str | None:
    """取模块名的顶层包名。codex.bridge -> codex。"""
    if not module:
        return None
    return module.split(".", 1)[0]


def _file_owner(file: Path) -> str | None:
    """判断文件属于哪个 monorepo 包。

    adapter/biz/.../foo.py -> "adapter"
    codex/foo.py -> "codex"
    """
    rel = file.relative_to(ROOT).as_posix()
    parts = rel.split("/")
    if not parts:
        return None
    top_dir = parts[0]
    if top_dir == "adapter":
        return "adapter"
    # agents 是顶层目录：codex/ goose/ claude_code/
    return top_dir if top_dir in AGENT_PKG else None


def _scan_file(file: Path, owner: str, result: ScanResult) -> None:
    try:
        tree = ast.parse(file.read_text(encoding="utf-8"), filename=str(file))
    except SyntaxError as err:
        print(f"WARN: syntax error in {file}: {err}", file=sys.stderr)
        return

    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                _check(file, node.lineno, owner, alias.name, result)
        elif isinstance(node, ast.ImportFrom):
            if node.level == 0 and node.module:
                _check(file, node.lineno, owner, node.module, result)


def _check(file: Path, line: int, owner: str, module: str, result: ScanResult) -> None:
    top = _top_pkg(module)
    if not top:
        return

    # Rule 1: adapter 不得 import codex/goose/claude_code
    if owner in ADAPTER_PKG and top in AGENT_PKG:
        result.violations.append(
            Violation(
                file=file,
                line=line,
                importer=owner,
                imported=module,
                rule="adapter-must-not-import-agents",
            )
        )

    # Rule 2: agents 互不依赖
    if owner in AGENT_PKG and top in AGENT_PKG and owner != top:
        result.violations.append(
            Violation(
                file=file,
                line=line,
                importer=owner,
                imported=module,
                rule=f"agents-must-not-cross-import ({owner} -> {top})",
            )
        )


def main() -> int:
    result = ScanResult()
    for py in ROOT.rglob("*.py"):
        if "/__pycache__/" in str(py):
            continue
        if "/node_modules/" in str(py):
            continue
        rel = py.relative_to(ROOT).as_posix()
        top_dir = rel.split("/")[0]
        if top_dir != "adapter" and top_dir not in AGENT_PKG:
            continue
        owner = _file_owner(py)
        if not owner:
            continue
        result.scanned += 1
        _scan_file(py, owner, result)

    print(f"Scanned {result.scanned} Python files under {ROOT}")
    if result.ok():
        print("OK: no monorepo layer violations")
        return 0

    print(f"FAIL: {len(result.violations)} violations\n")
    by_rule: dict[str, list[Violation]] = defaultdict(list)
    for v in result.violations:
        by_rule[v.rule].append(v)
    for rule, vs in by_rule.items():
        print(f"[{rule}] ({len(vs)} violations)")
        for v in vs:
            print(v.format())
    return 1


if __name__ == "__main__":
    sys.exit(main())
