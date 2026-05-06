package previewshare

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

func RenderPasswordPage(share *PreviewShare, errorMessage string) string {
	fileName := ""
	expiresAt := ""
	if share != nil {
		fileName = html.EscapeString(share.FileName)
		expiresAt = share.ExpiresAt.Local().Format(time.RFC3339)
	}
	errHTML := ""
	if errorMessage != "" {
		errHTML = `<div class="error">` + html.EscapeString(errorMessage) + `</div>`
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Enter Preview Password</title>
  <style>
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: linear-gradient(135deg, #f8fafc, #e2e8f0); min-height: 100vh; display: flex; align-items: center; justify-content: center; color: #0f172a; }
    .card { width: min(420px, calc(100vw - 32px)); background: #fff; border-radius: 20px; padding: 28px; box-shadow: 0 20px 60px rgba(15, 23, 42, 0.12); }
    h1 { margin: 0 0 12px; font-size: 24px; }
    p { margin: 0 0 16px; color: #475569; }
    label { display: block; margin-bottom: 8px; font-size: 14px; color: #334155; }
    input { width: 100%%; padding: 12px 14px; border-radius: 12px; border: 1px solid #cbd5e1; font-size: 16px; }
    button { width: 100%%; margin-top: 16px; padding: 12px 14px; border: 0; border-radius: 12px; background: #0f172a; color: #fff; font-size: 16px; cursor: pointer; }
    .meta { margin-bottom: 18px; font-size: 13px; color: #64748b; }
    .error { margin-bottom: 12px; padding: 10px 12px; border-radius: 10px; background: #fef2f2; color: #b91c1c; font-size: 14px; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Enter Preview Password</h1>
    <p>This document is available as a read-only preview.</p>
    <div class="meta">File: %s<br>Available until: %s</div>
    %s
    <form method="post" action="">
      <label for="password">Password</label>
      <input id="password" name="password" type="password" inputmode="numeric" autocomplete="one-time-code" required />
      <button type="submit">Open Preview</button>
    </form>
  </div>
</body>
</html>`, fileName, expiresAt, errHTML)
}

func RenderLoginPage(share *PreviewShare, loginURL, errorMessage string) string {
	fileName := ""
	expiresAt := ""
	if share != nil {
		fileName = html.EscapeString(share.FileName)
		expiresAt = share.ExpiresAt.Local().Format(time.RFC3339)
	}
	errHTML := ""
	if errorMessage != "" {
		errHTML = `<div class="error">` + html.EscapeString(errorMessage) + `</div>`
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Sign In To Open Preview</title>
  <style>
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: linear-gradient(135deg, #f8fafc, #e2e8f0); min-height: 100vh; display: flex; align-items: center; justify-content: center; color: #0f172a; }
    .card { width: min(440px, calc(100vw - 32px)); background: #fff; border-radius: 20px; padding: 28px; box-shadow: 0 20px 60px rgba(15, 23, 42, 0.12); }
    h1 { margin: 0 0 12px; font-size: 24px; }
    p { margin: 0 0 16px; color: #475569; }
    .meta { margin-bottom: 18px; font-size: 13px; color: #64748b; }
    .error { margin-bottom: 12px; padding: 10px 12px; border-radius: 10px; background: #fef2f2; color: #b91c1c; font-size: 14px; }
    .button { display: inline-flex; width: 100%%; justify-content: center; align-items: center; margin-top: 8px; padding: 12px 14px; border-radius: 12px; background: #0f172a; color: #fff; text-decoration: none; font-size: 16px; box-sizing: border-box; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Sign In To Open Preview</h1>
    <p>This preview requires a signed-in OfficeCLI account before access can continue.</p>
    <div class="meta">File: %s<br>Available until: %s</div>
    %s
    <a class="button" href="%s">Continue To Sign In</a>
  </div>
</body>
</html>`, fileName, expiresAt, errHTML, html.EscapeString(strings.TrimSpace(loginURL)))
}

func RenderReportPage(share *PreviewShare, reportHTML []byte) string {
	title := "OfficeCLI Report Preview"
	if share != nil && strings.TrimSpace(share.FileName) != "" {
		title = strings.TrimSpace(share.FileName)
	}
	reportJSON, _ := json.Marshal(string(reportHTML))
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    html, body { margin: 0; width: 100%%; height: 100%%; background: #0f172a; }
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    #report-frame { width: 100%%; height: 100%%; border: 0; background: #fff; display: block; }
    .loading { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; color: #e2e8f0; background: linear-gradient(180deg, #0f172a, #1e293b); }
  </style>
</head>
<body>
  <div class="loading" id="loading">Loading report preview...</div>
  <iframe
    id="report-frame"
    title="OfficeCLI Report Preview"
    sandbox="allow-scripts allow-downloads"
    referrerpolicy="no-referrer"
  ></iframe>
  <script>
    (function() {
      var frame = document.getElementById("report-frame");
      var loading = document.getElementById("loading");
      frame.srcdoc = %s;
      frame.addEventListener("load", function() {
        if (loading) loading.style.display = "none";
      }, { once: true });
    })();
  </script>
</body>
</html>`, html.EscapeString(title), string(reportJSON))
}

func RenderImagePage(share *PreviewShare, rawURL, downloadURL string) string {
	title := "OfficeCLI Image Preview"
	fileName := ""
	expiresAt := ""
	if share != nil {
		if strings.TrimSpace(share.FileName) != "" {
			title = strings.TrimSpace(share.FileName)
		}
		fileName = html.EscapeString(share.FileName)
		expiresAt = share.ExpiresAt.Local().Format(time.RFC3339)
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <style>
    body { margin: 0; min-height: 100vh; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #0f172a; color: #e2e8f0; }
    .shell { min-height: 100vh; display: grid; grid-template-rows: auto 1fr; }
    header { padding: 16px 20px; border-bottom: 1px solid rgba(148, 163, 184, 0.24); display: flex; align-items: center; justify-content: space-between; gap: 16px; }
    h1 { margin: 0; font-size: 18px; font-weight: 650; }
    .meta { margin-top: 4px; font-size: 13px; color: #94a3b8; }
    a { color: #fff; text-decoration: none; background: #2563eb; border-radius: 8px; padding: 9px 12px; white-space: nowrap; }
    main { min-height: 0; display: flex; align-items: center; justify-content: center; padding: 20px; }
    img { max-width: 100%%; max-height: calc(100vh - 118px); object-fit: contain; box-shadow: 0 24px 80px rgba(0, 0, 0, 0.35); background: #020617; }
  </style>
</head>
<body>
  <div class="shell">
    <header>
      <div>
        <h1>OfficeCLI Image Preview</h1>
        <div class="meta">File: %s<br>Available until: %s</div>
      </div>
      <a href="%s">Download</a>
    </header>
    <main>
      <img src="%s" alt="%s">
    </main>
  </div>
</body>
</html>`, html.EscapeString(title), fileName, expiresAt, html.EscapeString(strings.TrimSpace(downloadURL)), html.EscapeString(strings.TrimSpace(rawURL)), fileName)
}
