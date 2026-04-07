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
        "category": "公司介绍",
        "topic": "企业协作平台介绍",
        "brief": "为潜在企业客户介绍企业协作平台的核心能力、客户价值、典型场景与落地方式。",
        "audience": "潜在企业客户",
        "style": "专业克制",
        "lang": "zh-CN",
    },
    {
        "id": "market",
        "category": "行业/市场分析",
        "topic": "AI 办公出海市场机会分析",
        "brief": "面向管理层分析 AI 办公出海的市场空间、区域机会、竞争格局、进入策略与风险提示。",
        "audience": "管理层",
        "style": "结论先行",
        "lang": "zh-CN",
    },
    {
        "id": "ops",
        "category": "经营/数据汇报",
        "topic": "SaaS 季度经营复盘",
        "brief": "面向经营团队复盘季度收入、获客、留存、成本与下季度经营动作，要求数据驱动。",
        "audience": "经营团队",
        "style": "数据驱动",
        "lang": "zh-CN",
    },
    {
        "id": "launch",
        "category": "产品发布方案",
        "topic": "OfficeCLI 新版本发布方案",
        "brief": "面向跨部门项目组给出新版本发布目标、节奏、资源分工、风险预案与里程碑。",
        "audience": "跨部门项目组",
        "style": "行动导向",
        "lang": "zh-CN",
    },
    {
        "id": "training",
        "category": "培训/教程",
        "topic": "OfficeCLI 新员工上手培训",
        "brief": "面向新员工说明 OfficeCLI 的定位、常用命令、典型流程、注意事项与上手建议。",
        "audience": "新员工",
        "style": "清晰易学",
        "lang": "zh-CN",
    },
]

SUITES = {
    "smoke": ["launch"],
    "full": [item["id"] for item in CASES],
}

PASS_SCORE = 85


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
        raise ValueError("--bin 不能为空")
    return parts


def build_summary_markdown(report):
    lines = [
        "# PPT 质量评测汇总",
        "",
        f"- 套件：{report['suite']}",
        f"- 安装版本：{report['installed_version']}",
        f"- 整体状态：{report['status']}",
        f"- 样例数：{len(report['cases'])}",
        "",
        "| 样例 | 发布 | 总分 | 视觉分 | 结构分 | high | medium | 结果 | 在线预览 | 关键问题 |",
        "| --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- | --- |",
    ]
    for item in report["cases"]:
        issues = "；".join(item["top_issues"]) if item["top_issues"] else "-"
        preview = item.get("access_url") or "-"
        publish_state = "已发布" if item.get("published") else "未发布"
        result = "通过" if item["passed"] else "未通过"
        lines.append(
            f"| {item['category']} | {publish_state} | {item['overall_score']} | {item['visual_score']} | {item['structure_score']} | "
            f"{item['high_count']} | {item['medium_count']} | {result} | {preview} | {issues} |"
        )
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description="批量生成并评审固定 PPT 质量样例。")
    parser.add_argument("--date", default="2026-04-07", help="输出日期目录，例如 2026-04-07")
    parser.add_argument("--round", dest="round_name", default="baseline", help="轮次名称，例如 baseline / round-1")
    parser.add_argument("--suite", choices=sorted(SUITES.keys()), default="full", help="评测套件")
    parser.add_argument("--bin", default="go run ./cmd/officecli", help="officecli 执行命令")
    parser.add_argument("--root", default=".", help="仓库根目录")
    parser.add_argument("--publish", action="store_true", help="生成后发布在线预览")
    parser.add_argument("--version-label", default="unknown", help="安装版本标识")
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
        write_blocker("未找到 soffice，无法执行 PDF 视觉评审。", "")
        print("阻塞：未找到 soffice，无法执行 PDF 视觉评审。", file=sys.stderr)
        return 2

    bin_cmd = normalize_bin_cmd(args.bin)
    config_status = run_text(bin_cmd + ["config", "status"], repo)
    if "生成服务已配置：true" not in config_status:
        write_blocker("当前 generation 服务未配置，无法执行真实生成闭环。", config_status.strip())
        print("阻塞：当前 generation 服务未配置，无法执行真实生成闭环。", file=sys.stderr)
        print(config_status.strip(), file=sys.stderr)
        return 2
    if args.publish and "在线预览发布已启用：true" not in config_status:
        write_blocker("当前在线预览发布未启用，无法验证 ClaudeOffice 预览地址。", config_status.strip())
        print("阻塞：当前在线预览发布未启用。", file=sys.stderr)
        print(config_status.strip(), file=sys.stderr)
        return 2

    cases = select_cases(args.suite)
    report = {
        "status": "passed",
        "suite": args.suite,
        "installed_version": args.version_label,
        "pass_score": PASS_SCORE,
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
            passed = review.get("overall_score", 0) >= PASS_SCORE and high_count == 0 and publish_ok
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
                    "passed": passed,
                    "top_issues": top_issues,
                    "published_skipped_reason": generate.get("published_skipped_reason", ""),
                    "review_summary": review.get("summary", ""),
                }
            )
    except Exception as exc:
        write_blocker("生成或 review 链路执行失败。", str(exc))
        print(f"阻塞：{exc}", file=sys.stderr)
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
