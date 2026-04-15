#!/usr/bin/env python3

import argparse
import json
import shlex
import shutil
import subprocess
import sys
from pathlib import Path

CASES = [
    {
        "id": "company",
        "category": "Company Overview",
        "topic": "Enterprise Collaboration Platform Overview",
        "brief": "Introduce the core capabilities, customer value, typical scenarios, and rollout path of an enterprise collaboration platform to prospective enterprise customers.",
        "audience": "Prospective enterprise customers",
        "style": "Professional and restrained",
        "lang": "en-US",
    },
    {
        "id": "market",
        "category": "Industry / Market Analysis",
        "topic": "Global Expansion Opportunity Analysis for AI Productivity",
        "brief": "Analyze market size, regional opportunities, competitive landscape, market-entry strategy, and key risks for AI productivity going global, aimed at leadership.",
        "audience": "Leadership",
        "style": "Conclusion first",
        "lang": "en-US",
    },
    {
        "id": "ops",
        "category": "Operations / Data Review",
        "topic": "SaaS Quarterly Business Review",
        "brief": "Review quarterly revenue, acquisition, retention, cost structure, and next-quarter operating actions for the operating team with a data-driven narrative.",
        "audience": "Operations team",
        "style": "Data-driven",
        "lang": "en-US",
    },
    {
        "id": "launch",
        "category": "Product Launch Plan",
        "topic": "OfficeCLI New Release Plan",
        "brief": "Provide release goals, cadence, ownership split, risk mitigation, and milestones for a cross-functional launch team.",
        "audience": "Cross-functional launch team",
        "style": "Action-oriented",
        "lang": "en-US",
    },
    {
        "id": "training",
        "category": "Training / Tutorial",
        "topic": "OfficeCLI New Hire Onboarding",
        "brief": "Explain positioning, common commands, standard workflows, key cautions, and onboarding advice for new hires using OfficeCLI.",
        "audience": "New hires",
        "style": "Clear and easy to learn",
        "lang": "en-US",
    },
    {
        "id": "company_exec",
        "category": "Company Overview",
        "topic": "Enterprise Collaboration Platform for Leadership",
        "brief": "Explain business value, governance capabilities, ROI, and phased rollout recommendations for an enterprise leadership audience.",
        "audience": "Enterprise leadership",
        "style": "Conclusion first",
        "lang": "en-US",
    },
    {
        "id": "market_finance",
        "category": "Industry / Market Analysis",
        "topic": "AI Productivity Competitive Landscape and Entry Window",
        "brief": "Analyze the competitive landscape, entry window, regional priorities, and risk boundaries of the AI productivity market for investment and strategy teams.",
        "audience": "Investment and strategy team",
        "style": "Conclusion first",
        "lang": "en-US",
    },
    {
        "id": "ops_board",
        "category": "Operations / Data Review",
        "topic": "SaaS Monthly Operating Dashboard Review",
        "brief": "Review monthly new business, renewals, collections, delivery efficiency, and next-month corrective actions for operating leadership with a data-driven narrative.",
        "audience": "Operating leadership",
        "style": "Data-driven",
        "lang": "en-US",
    },
    {
        "id": "launch_gtm",
        "category": "Product Launch Plan",
        "topic": "OfficeCLI New Release GTM Coordination Plan",
        "brief": "Define launch cadence, ownership split, contingency plans, and post-launch tracking for product, sales, operations, and support teams.",
        "audience": "Product, sales, operations, and support team",
        "style": "Action-oriented",
        "lang": "en-US",
    },
    {
        "id": "training_ops",
        "category": "Training / Tutorial",
        "topic": "OfficeCLI Daily Usage Basics",
        "brief": "Teach frontline users the install checks, common commands, generate/review workflow, and common pitfalls of OfficeCLI.",
        "audience": "Frontline business users",
        "style": "Clear and easy to learn",
        "lang": "en-US",
    },
]

SUITES = {
    "smoke": ["launch"],
    "full": [item["id"] for item in CASES[:5]],
    "extended": [item["id"] for item in CASES],
    "strict": [item["id"] for item in CASES],
}

DEFAULT_PASS_SCORE = 85
STRICT_PASS_SCORE = 88
STRICT_VISUAL_SCORE = 80
STRICT_MAX_MEDIUM = 2


def run_json(cmd, cwd):
    proc = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip() or f"command failed: {' '.join(cmd)}")
    return json.loads(proc.stdout)


def run_text(cmd, cwd):
    proc = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True)
    if proc.returncode != 0:
        raise RuntimeError(proc.stderr.strip() or proc.stdout.strip() or f"command failed: {' '.join(cmd)}")
    return proc.stdout


def write_json(path, payload):
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def select_cases(suite):
    allowed = set(SUITES[suite])
    return [case for case in CASES if case["id"] in allowed]


def normalize_bin_cmd(value):
    parts = shlex.split(value)
    if not parts:
        raise ValueError("--bin cannot be empty")
    return parts


def build_summary_markdown(report):
    lines = [
        "# PPT Quality Evaluation Summary",
        "",
        f"- Suite: {report['suite']}",
        f"- Installed version: {report['installed_version']}",
        f"- Overall status: {report['status']}",
        f"- Case count: {len(report['cases'])}",
        f"- Pass threshold: overall >= {report['pass_score']} / visual >= {report['min_visual_score']} / medium <= {report['max_medium_count']} / high must be 0",
        "",
        "| Case | Published | Overall | Visual | Structure | high | medium | Result | Visual Review | Online Preview | Top Issues |",
        "| --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- | --- | --- |",
    ]
    for item in report["cases"]:
        issues = "; ".join(item["top_issues"]) if item["top_issues"] else "-"
        preview = item.get("access_url") or "-"
        publish_state = "yes" if item.get("published") else "no"
        result = "passed" if item["passed"] else "failed"
        visual = "used" if item.get("used_visual") else "skipped"
        lines.append(
            f"| {item['category']} | {publish_state} | {item['overall_score']} | {item['visual_score']} | {item['structure_score']} | "
            f"{item['high_count']} | {item['medium_count']} | {result} | {visual} | {preview} | {issues} |"
        )
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description="Generate and review a fixed set of PPT quality cases in batch.")
    parser.add_argument("--date", default="2026-04-07", help="Output date directory, for example 2026-04-07")
    parser.add_argument("--round", dest="round_name", default="baseline", help="Round name, for example baseline or round-1")
    parser.add_argument("--suite", choices=sorted(SUITES.keys()), default="full", help="Evaluation suite")
    parser.add_argument("--bin", default="go run ./cmd/officecli", help="officecli execution command")
    parser.add_argument("--root", default=".", help="Repository root")
    parser.add_argument("--publish", action="store_true", help="Publish online previews after generation")
    parser.add_argument("--version-label", default="unknown", help="Installed version label")
    parser.add_argument("--pass-score", type=int, default=None, help="Minimum overall score required to pass")
    parser.add_argument("--min-visual-score", type=int, default=None, help="Minimum visual score required to pass")
    parser.add_argument("--max-medium-count", type=int, default=None, help="Maximum allowed medium issue count")
    parser.add_argument("--require-visual", action="store_true", help="Require visual review for every case")
    args = parser.parse_args()

    repo = Path(args.root).resolve()
    out_root = repo / "output" / "evals" / args.date / args.round_name
    out_root.mkdir(parents=True, exist_ok=True)
    blocked_path = out_root / "blocked.json"
    summary_path = out_root / "summary.json"
    summary_md_path = out_root / "summary.md"
    if blocked_path.exists():
        blocked_path.unlink()

    def write_blocker(message, details):
        payload = {
            "status": "blocked",
            "suite": args.suite,
            "installed_version": args.version_label,
            "message": message,
            "details": details,
        }
        write_json(blocked_path, payload)

    if shutil.which("soffice") is None:
        write_blocker("soffice was not found, so PDF visual review cannot run.", "")
        print("Blocked: soffice was not found, so PDF visual review cannot run.", file=sys.stderr)
        return 2

    bin_cmd = normalize_bin_cmd(args.bin)
    config_status = run_text(bin_cmd + ["config", "status"], repo)
    if "Generation service configured: true" not in config_status:
        write_blocker("The generation service is not configured, so the end-to-end generation flow cannot run.", config_status.strip())
        print("Blocked: the generation service is not configured, so the end-to-end generation flow cannot run.", file=sys.stderr)
        print(config_status.strip(), file=sys.stderr)
        return 2
    if args.publish and "Online preview publishing enabled: true" not in config_status:
        write_blocker("Online preview publishing is not enabled, so the OfficeCLI preview URL cannot be validated.", config_status.strip())
        print("Blocked: online preview publishing is not enabled.", file=sys.stderr)
        print(config_status.strip(), file=sys.stderr)
        return 2

    cases = select_cases(args.suite)
    pass_score = args.pass_score
    min_visual_score = args.min_visual_score
    max_medium_count = args.max_medium_count
    require_visual = args.require_visual
    if args.suite == "strict":
        if pass_score is None:
            pass_score = STRICT_PASS_SCORE
        if min_visual_score is None:
            min_visual_score = STRICT_VISUAL_SCORE
        if max_medium_count is None:
            max_medium_count = STRICT_MAX_MEDIUM
        require_visual = True
    else:
        if pass_score is None:
            pass_score = DEFAULT_PASS_SCORE
        if min_visual_score is None:
            min_visual_score = 0
        if max_medium_count is None:
            max_medium_count = 999

    report = {
        "status": "passed",
        "suite": args.suite,
        "installed_version": args.version_label,
        "pass_score": pass_score,
        "min_visual_score": min_visual_score,
        "max_medium_count": max_medium_count,
        "require_visual": require_visual,
        "publish_enabled": args.publish,
        "output_dir": str(out_root),
        "cases": [],
    }

    try:
        for case in cases:
            case_dir = out_root / case["id"]
            case_dir.mkdir(parents=True, exist_ok=True)

            generate_cmd = (
                bin_cmd
                + [
                    "new",
                    "pptx",
                    case["topic"],
                    case["brief"],
                    "--mode",
                    "fast",
                    "--audience",
                    case["audience"],
                    "--style",
                    case["style"],
                    "--lang",
                    case["lang"],
                    "--json",
                    "--out",
                    str(case_dir),
                ]
            )
            if not args.publish:
                generate_cmd.append("--no-publish")

            generate = run_json(generate_cmd, repo)
            review = run_json(bin_cmd + ["review", "pptx", generate["file_path"], "--json"], repo)

            write_json(case_dir / "generate.json", generate)
            write_json(case_dir / "review.json", review)

            high_count = sum(1 for item in review.get("issues", []) if item.get("severity") == "high")
            medium_count = sum(1 for item in review.get("issues", []) if item.get("severity") == "medium")
            top_issues = [item.get("title", "") for item in review.get("issues", [])[:3] if item.get("title")]
            published = bool(generate.get("published"))
            preview_url = (generate.get("access_url") or "").strip()
            publish_ok = (not args.publish) or (published and preview_url)
            used_visual = bool(review.get("used_visual"))
            visual_ok = review.get("visual_score", 0) >= min_visual_score
            medium_ok = medium_count <= max_medium_count
            visual_required_ok = (not require_visual) or used_visual
            passed = (
                review.get("overall_score", 0) >= pass_score
                and visual_ok
                and high_count == 0
                and medium_ok
                and publish_ok
                and visual_required_ok
            )
            if not passed:
                report["status"] = "failed"

            report["cases"].append(
                {
                    "id": case["id"],
                    "category": case["category"],
                    "topic": case["topic"],
                    "pptx": generate["file_path"],
                    "installed_version": args.version_label,
                    "published": published,
                    "access_url": preview_url,
                    "password": generate.get("password", ""),
                    "expires_at": generate.get("expires_at", ""),
                    "overall_score": review.get("overall_score", 0),
                    "visual_score": review.get("visual_score", 0),
                    "structure_score": review.get("structure_score", 0),
                    "high_count": high_count,
                    "medium_count": medium_count,
                    "used_visual": used_visual,
                    "passed": passed,
                    "top_issues": top_issues,
                    "published_skipped_reason": generate.get("published_skipped_reason", ""),
                    "review_summary": review.get("summary", ""),
                }
            )
    except Exception as exc:
        write_blocker("The generate or review flow failed.", str(exc))
        print(f"Blocked: {exc}", file=sys.stderr)
        return 2

    write_json(summary_path, report)
    summary_md_path.write_text(build_summary_markdown(report), encoding="utf-8")

    print(json.dumps({
        "output_dir": str(out_root),
        "status": report["status"],
        "suite": report["suite"],
        "installed_version": report["installed_version"],
        "failed": report["status"] != "passed",
    }, ensure_ascii=False))
    return 1 if report["status"] != "passed" else 0


if __name__ == "__main__":
    sys.exit(main())
