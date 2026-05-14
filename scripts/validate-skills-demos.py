#!/usr/bin/env python3

from __future__ import annotations

import json
import sys
from pathlib import Path


REQUIRED_DEMOS = {
    "pptx-image-rich": ".pptx",
    "pptx-text-only": ".pptx",
    "docx-brief": ".docx",
    "xlsx-dashboard": ".xlsx",
    "report-workbook": ".html",
    "standalone-img": ".png",
}
REQUIRED_METADATA_KEYS = {
    "title",
    "type",
    "command",
    "artifact",
    "preview",
    "prompt_file",
    "verified_at",
}
MAX_ARTIFACT_BYTES = 3 * 1024 * 1024


def fail(message: str) -> None:
    print(f"ERROR: {message}", file=sys.stderr)
    raise SystemExit(1)


def is_png(data: bytes) -> bool:
    return data.startswith(b"\x89PNG\r\n\x1a\n")


def is_jpeg(data: bytes) -> bool:
    return data.startswith(b"\xff\xd8\xff")


def validate(root: Path) -> None:
    if not root.is_dir():
        fail(f"demo root not found: {root}")

    missing = sorted(set(REQUIRED_DEMOS) - {item.name for item in root.iterdir() if item.is_dir()})
    if missing:
        fail(f"missing demo directories: {', '.join(missing)}")

    for slug, expected_suffix in REQUIRED_DEMOS.items():
        demo_dir = root / slug
        meta_path = demo_dir / "metadata.json"
        if not meta_path.is_file():
            fail(f"{slug}: missing metadata.json")

        metadata = json.loads(meta_path.read_text())
        missing_keys = sorted(REQUIRED_METADATA_KEYS - metadata.keys())
        if missing_keys:
            fail(f"{slug}: metadata missing keys: {', '.join(missing_keys)}")

        artifact = demo_dir / metadata["artifact"]
        preview = demo_dir / metadata["preview"]
        prompt = demo_dir / metadata["prompt_file"]
        for path in (artifact, preview, prompt):
            if not path.is_file():
                fail(f"{slug}: missing file {path.name}")

        if artifact.suffix != expected_suffix:
            fail(f"{slug}: artifact suffix {artifact.suffix!r}, expected {expected_suffix!r}")
        if artifact.stat().st_size > MAX_ARTIFACT_BYTES:
            fail(f"{slug}: artifact exceeds 3MB: {artifact.stat().st_size}")

        preview_bytes = preview.read_bytes()
        if not (is_png(preview_bytes) or is_jpeg(preview_bytes)):
            fail(f"{slug}: preview is not a PNG or JPEG")

        if artifact.suffix in {".pptx", ".docx", ".xlsx"} and not artifact.read_bytes().startswith(b"PK\x03\x04"):
            fail(f"{slug}: Office artifact is not a zip package")
        if artifact.suffix == ".html" and b"<html" not in artifact.read_bytes().lower():
            fail(f"{slug}: report artifact is not HTML")
        if artifact.suffix == ".png" and not is_png(artifact.read_bytes()):
            fail(f"{slug}: image artifact is not PNG")

        command = metadata["command"]
        if "officecli new" not in command:
            fail(f"{slug}: command does not show officecli new usage")

        if slug == "report-workbook":
            source = metadata.get("additional_files", {}).get("source_workbook")
            if not source or not (demo_dir / source).is_file():
                fail(f"{slug}: missing source workbook reference")

    print(f"validated {len(REQUIRED_DEMOS)} skills demos in {root}")


if __name__ == "__main__":
    validate(Path(sys.argv[1]) if len(sys.argv) > 1 else Path("public/skills-demos"))
