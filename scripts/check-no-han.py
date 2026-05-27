#!/usr/bin/env python3

from __future__ import annotations

import subprocess
import sys
from pathlib import Path


EXCLUDED_PARTS = {
    "dist",
    "node_modules",
    ".git",
}

EXCLUDED_PATHS = {
    "AGENTS.md",
    "CHANGELOG.md",
}

EXCLUDED_PREFIXES = (
    "platform/web/admin/",
)

EXCLUDED_SUFFIXES = {
    ".go",
    ".png",
    ".jpg",
    ".jpeg",
    ".gif",
    ".webp",
    ".svg",
    ".pdf",
    ".pptx",
    ".docx",
    ".xlsx",
    ".zip",
    ".gz",
    ".tar",
    ".ico",
    ".woff",
    ".woff2",
    ".ttf",
    ".otf",
    ".mp4",
    ".mov",
    ".sh",
}


def is_han(ch: str) -> bool:
    code = ord(ch)
    return (
        0x3400 <= code <= 0x4DBF
        or 0x4E00 <= code <= 0x9FFF
        or 0xF900 <= code <= 0xFAFF
        or 0x20000 <= code <= 0x2A6DF
        or 0x2A700 <= code <= 0x2B73F
        or 0x2B740 <= code <= 0x2B81F
        or 0x2B820 <= code <= 0x2CEAF
        or 0x2CEB0 <= code <= 0x2EBEF
        or 0x30000 <= code <= 0x3134F
    )


def tracked_files() -> list[Path]:
    result = subprocess.run(
        ["git", "ls-files"],
        check=True,
        capture_output=True,
        text=True,
    )
    out: list[Path] = []
    for raw in result.stdout.splitlines():
        path = Path(raw)
        normalized = path.as_posix()
        if normalized in EXCLUDED_PATHS:
            continue
        if any(normalized.startswith(prefix) for prefix in EXCLUDED_PREFIXES):
            continue
        if path.parts and path.parts[0] == "docs":
            continue
        if any(part in EXCLUDED_PARTS for part in path.parts):
            continue
        if path.suffix.lower() in EXCLUDED_SUFFIXES:
            continue
        out.append(path)
    return out


def first_han_line(path: Path) -> tuple[int, str] | None:
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return None
    except OSError:
        return None

    for lineno, line in enumerate(text.splitlines(), start=1):
        if any(is_han(ch) for ch in line):
            return lineno, line.strip()
    return None


def main() -> int:
    violations: list[str] = []
    for path in tracked_files():
        hit = first_han_line(path)
        if hit is None:
            continue
        lineno, line = hit
        violations.append(f"{path}:{lineno}: {line}")

    if not violations:
        print("No Han characters found in tracked text files.")
        return 0

    print("Han characters are not allowed in tracked text files:")
    for item in violations:
        print(item)
    return 1


if __name__ == "__main__":
    sys.exit(main())
