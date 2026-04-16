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
