package officesdk

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	FileTypeDocument     = 0
	FileTypeSpreadsheet  = 1
	FileTypePresentation = 2
	defaultSDKPathPrefix = "/sdk/turbo-ai"
)

func GetFileTypeEnum(filename string) int {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".doc", ".docx":
		return FileTypeDocument
	case ".xls", ".xlsx":
		return FileTypeSpreadsheet
	case ".ppt", ".pptx":
		return FileTypePresentation
	default:
		return -1
	}
}

func ExtractPathPrefix(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	idx := strings.Index(endpoint, "://")
	if idx < 0 {
		return ""
	}
	rest := endpoint[idx+3:]
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return ""
	}
	return rest[slashIdx:]
}

func ExtractSDKAppID(endpoint string) string {
	pathPrefix := strings.Trim(ExtractPathPrefix(endpoint), "/")
	if pathPrefix == "" {
		return ""
	}
	parts := strings.Split(pathPrefix, "/")
	return parts[len(parts)-1]
}

func ResolveSDKFilePagePath(endpoint string) string {
	pathPrefix := ExtractPathPrefix(endpoint)
	if pathPrefix == "" {
		pathPrefix = defaultSDKPathPrefix
	}
	return pathPrefix + "/v1/api/file/page"
}

func RenderOfficePage(officesdkEndpoint, fileID, token string, fileType int, readonly bool) string {
	role := "viewer"
	if !readonly {
		role = "editor"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OfficeCLI Preview</title>
  <style>
    * { box-sizing: border-box; }
    html, body { width: 100%%; height: 100%%; margin: 0; background: #f4f1e8; color: #1f2937; }
    body { font-family: "Segoe UI", system-ui, sans-serif; }
    #root { width: 100%%; height: 100%%; }
    .loading, .error { display: flex; width: 100%%; height: 100%%; align-items: center; justify-content: center; padding: 24px; text-align: center; }
    .loading { color: #475569; }
    .error { color: #b91c1c; }
  </style>
</head>
<body>
  <div id="root">
    <div class="loading" id="loading">Loading preview...</div>
  </div>
  <script src="https://unpkg.com/@officesdk/web@1.0.9/umd/index.js"></script>
  <script>
    (function() {
      var endpoint = %q;
      var sdkPath = %q;
      var fileId = %q;
      var token = %q;
      var fileType = %d;
      var role = %q;
      function showError(message) {
        document.getElementById("root").innerHTML = '<div class="error">' + message + '</div>';
      }
      function init() {
        var pkg = window["@officesdk/web"];
        if (!pkg || typeof pkg.createSDK !== "function") {
          showError("Failed to load the OfficeSDK JavaScript SDK.");
          return;
        }
        document.getElementById("loading").style.display = "none";
        var sdk = pkg.createSDK({
          endpoint: endpoint,
          path: sdkPath,
          fileId: fileId,
          fileType: fileType,
          token: token,
          lang: "en-US",
          root: document.getElementById("root"),
          mode: "standard",
          role: role
        });
        sdk.connect().catch(function(err) {
          showError("Failed to load document: " + ((err && err.message) || String(err)));
        });
      }
      if (document.readyState === "complete") {
        init();
        return;
      }
      window.addEventListener("load", init);
    })();
  </script>
</body>
</html>`, officesdkEndpoint, ResolveSDKFilePagePath(officesdkEndpoint), fileID, token, fileType, role)
}
