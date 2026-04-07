#!/usr/bin/env python3

import argparse
import json
import os
import smtplib
import sys
import urllib.request
from email.message import EmailMessage
from pathlib import Path


def load_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def summarize_status(report_root):
    blocked_path = report_root / "blocked.json"
    summary_path = report_root / "summary.json"
    if blocked_path.exists():
        blocked = load_json(blocked_path)
        return {
            "status": "blocked",
            "suite": blocked.get("suite", "unknown"),
            "installed_version": blocked.get("installed_version", "unknown"),
            "message": blocked.get("message", "阻塞"),
            "details": blocked.get("details", ""),
            "cases": [],
        }
    if not summary_path.exists():
        raise FileNotFoundError(f"summary.json not found in {report_root}")
    return load_json(summary_path)


def pick_case(report):
    cases = report.get("cases") or []
    return cases[0] if cases else None


def summarize_cases(report):
    cases = report.get("cases") or []
    lines = []
    for case in cases:
        lines.append(
            f"- {case.get('category', '-')}: score={case.get('overall_score', 0)} "
            f"high={case.get('high_count', 0)} medium={case.get('medium_count', 0)} "
            f"preview={case.get('access_url') or 'preview unavailable'}"
        )
    return lines


def build_subject(report):
    status = report.get("status", "unknown").upper()
    suite = report.get("suite", "unknown")
    version = report.get("installed_version", "unknown")
    return f"[officecli][installed-e2e][{status}] suite={suite} version={version}"


def build_text(report, run_url):
    lines = [
        "OfficeCLI 安装式 E2E 定时测试结果",
        f"状态: {report.get('status', 'unknown')}",
        f"套件: {report.get('suite', 'unknown')}",
        f"安装版本: {report.get('installed_version', 'unknown')}",
    ]
    if run_url:
        lines.append(f"运行地址: {run_url}")

    if report.get("status") == "blocked":
        lines.append(f"阻塞原因: {report.get('message', '-')}")
        details = (report.get("details") or "").strip()
        if details:
            lines.append(f"详情: {details}")
        return "\n".join(lines)

    case = pick_case(report)
    cases = report.get("cases") or []
    if case is None:
        lines.append("未找到案例结果，请查看 workflow artifact。")
        return "\n".join(lines)

    lines.extend(
        [
            f"案例数量: {len(cases)}",
            f"主案例: {case.get('category', '-')}/{case.get('topic', '-')}",
            f"主案例总分: {case.get('overall_score', 0)}",
            f"主案例视觉分: {case.get('visual_score', 0)}",
            f"主案例结构分: {case.get('structure_score', 0)}",
            f"主案例 High 问题数: {case.get('high_count', 0)}",
            f"主案例 Medium 问题数: {case.get('medium_count', 0)}",
            f"主案例在线预览: {case.get('access_url') or 'preview unavailable'}",
            f"主案例访问密码: {case.get('password') or '-'}",
            f"主案例过期时间: {case.get('expires_at') or '-'}",
        ]
    )
    lines.append("案例汇总:")
    lines.extend(summarize_cases(report))
    top_issues = case.get("top_issues") or []
    if top_issues:
        lines.append("关键问题: " + "；".join(top_issues[:3]))
    review_summary = (case.get("review_summary") or "").strip()
    if review_summary:
        lines.append(f"评审结论: {review_summary}")
    return "\n".join(lines)


def build_dingtalk_text(report, run_url):
    base = ["officecli 安装式 E2E"]
    base.append(f"状态: {report.get('status', 'unknown')}")
    base.append(f"套件: {report.get('suite', 'unknown')}")
    base.append(f"版本: {report.get('installed_version', 'unknown')}")
    case = pick_case(report)
    if case is not None:
        base.append(f"主案例总分: {case.get('overall_score', 0)}")
        base.append(f"主案例预览: {case.get('access_url') or 'preview unavailable'}")
        extra = summarize_cases(report)
        if extra:
            base.append("案例汇总:")
            base.extend(extra[:5])
    if report.get("status") == "blocked":
        base.append(f"阻塞: {report.get('message', '-')}")
    if run_url:
        base.append(f"Run: {run_url}")
    return "\n".join(base)


def send_email(report, run_url):
    host = os.environ.get("CLI_E2E_EMAIL_SMTP_HOST", "").strip()
    port = int(os.environ.get("CLI_E2E_EMAIL_SMTP_PORT", "587").strip() or "587")
    username = os.environ.get("CLI_E2E_EMAIL_SMTP_USERNAME", "").strip()
    password = os.environ.get("CLI_E2E_EMAIL_SMTP_PASSWORD", "").strip()
    sender = os.environ.get("CLI_E2E_EMAIL_FROM", "").strip()
    recipients = [item.strip() for item in os.environ.get("CLI_E2E_EMAIL_TO", "").split(",") if item.strip()]
    missing = [
        name
        for name, value in [
            ("CLI_E2E_EMAIL_SMTP_HOST", host),
            ("CLI_E2E_EMAIL_SMTP_USERNAME", username),
            ("CLI_E2E_EMAIL_SMTP_PASSWORD", password),
            ("CLI_E2E_EMAIL_FROM", sender),
            ("CLI_E2E_EMAIL_TO", ",".join(recipients)),
        ]
        if not value
    ]
    if missing:
        raise RuntimeError("missing email config: " + ", ".join(missing))

    message = EmailMessage()
    message["Subject"] = build_subject(report)
    message["From"] = sender
    message["To"] = ", ".join(recipients)
    message.set_content(build_text(report, run_url))

    with smtplib.SMTP(host, port, timeout=30) as smtp:
        smtp.starttls()
        smtp.login(username, password)
        smtp.send_message(message)


def send_dingtalk(report, run_url):
    webhook = os.environ.get("DINGTALK_WEBHOOK", "").strip()
    if not webhook:
        raise RuntimeError("missing DINGTALK_WEBHOOK")
    payload = json.dumps(
        {
            "msgtype": "text",
            "text": {
                "content": build_dingtalk_text(report, run_url),
            },
        },
        ensure_ascii=False,
    ).encode("utf-8")
    request = urllib.request.Request(
        webhook,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        body = response.read().decode("utf-8", errors="replace")
        if response.status >= 300:
            raise RuntimeError(f"dingtalk http status={response.status} body={body}")


def main():
    parser = argparse.ArgumentParser(description="发送 officecli 安装式 E2E 邮件与钉钉通知")
    parser.add_argument("report_root", help="评测结果目录")
    parser.add_argument("--run-url", default=os.environ.get("RUN_URL", ""), help="GitHub Actions run 链接")
    args = parser.parse_args()

    report = summarize_status(Path(args.report_root).resolve())
    send_email(report, args.run_url)
    send_dingtalk(report, args.run_url)
    print(json.dumps({"status": report.get("status", "unknown")}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"notify failed: {exc}", file=sys.stderr)
        raise
