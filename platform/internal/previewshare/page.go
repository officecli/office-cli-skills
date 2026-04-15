package previewshare

import "fmt"

func RenderLoginRequiredPage(loginURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Sign in to view preview</title>
  <style>
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: linear-gradient(135deg, #f8fafc, #e2e8f0); min-height: 100vh; display: flex; align-items: center; justify-content: center; color: #0f172a; }
    .card { width: min(460px, calc(100vw - 32px)); background: white; border-radius: 24px; padding: 32px; box-shadow: 0 20px 60px rgba(15, 23, 42, 0.12); }
    h1 { margin: 0 0 12px; font-size: 28px; }
    p { margin: 0 0 18px; color: #475569; line-height: 1.6; }
    a { display: inline-flex; align-items: center; justify-content: center; min-width: 180px; padding: 12px 18px; border-radius: 14px; background: #0f172a; color: #fff; text-decoration: none; font-weight: 600; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Sign in required</h1>
    <p>This document preview is available only to OfficeCLI accounts that have completed Google sign-in. After signing in, you will be returned to this preview automatically.</p>
    <a href="%s">Sign in with Google</a>
  </div>
</body>
</html>`, loginURL)
}
