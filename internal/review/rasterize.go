package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type visualPageImage struct {
	Page int
	MIME string
	Data []byte
}

type rasterizeManifest struct {
	Pages []struct {
		Page int    `json:"page"`
		Path string `json:"path"`
		MIME string `json:"mime"`
	} `json:"pages"`
}

func rasterizePDFPagesWithSoffice(ctx context.Context, pdfPath string, maxPages int) ([]visualPageImage, error) {
	if maxPages <= 0 {
		maxPages = 8
	}
	absPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve PDF path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	workDir, err := os.MkdirTemp("", "officecli-review-raster-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	manifestPath := filepath.Join(workDir, "manifest.json")
	args := []string{
		"-c",
		sofficeRasterizePythonScript,
		absPath,
		workDir,
		strconv.Itoa(maxPages),
		manifestPath,
	}
	cmd := exec.CommandContext(ctx, "python3", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("LibreOffice page rasterization failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read rasterization manifest: %w", err)
	}
	var manifest rasterizeManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse rasterization manifest: %w", err)
	}
	if len(manifest.Pages) == 0 {
		return nil, fmt.Errorf("no page images were exported")
	}
	sort.SliceStable(manifest.Pages, func(i, j int) bool {
		return manifest.Pages[i].Page < manifest.Pages[j].Page
	})

	out := make([]visualPageImage, 0, len(manifest.Pages))
	for _, item := range manifest.Pages {
		imageBytes, err := os.ReadFile(item.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read page image: %w", err)
		}
		mime := strings.TrimSpace(item.MIME)
		if mime == "" {
			mime = "image/png"
		}
		out = append(out, visualPageImage{
			Page: item.Page,
			MIME: mime,
			Data: imageBytes,
		})
	}
	return out, nil
}

const sofficeRasterizePythonScript = `
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time

import uno
from com.sun.star.beans import PropertyValue


def prop(name, value):
    item = PropertyValue()
    item.Name = name
    item.Value = value
    return item


def free_port():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port


pdf_path = os.path.abspath(sys.argv[1])
output_dir = os.path.abspath(sys.argv[2])
max_pages = int(sys.argv[3])
manifest_path = os.path.abspath(sys.argv[4])
profile_dir = tempfile.mkdtemp(prefix="officecli-review-uno-profile-")
port = free_port()
accept = "socket,host=127.0.0.1,port=%d;urp;StarOffice.ComponentContext" % port
cmd = [
    "soffice",
    "--headless",
    "--invisible",
    "--norestore",
    "--nodefault",
    "--nolockcheck",
    "--nofirststartwizard",
    "--accept=%s" % accept,
    "-env:UserInstallation=file://%s" % profile_dir,
]
proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)

try:
    local_ctx = uno.getComponentContext()
    resolver = local_ctx.ServiceManager.createInstanceWithContext("com.sun.star.bridge.UnoUrlResolver", local_ctx)
    ctx = None
    last_err = None
    for _ in range(100):
        try:
            ctx = resolver.resolve("uno:%s" % accept)
            break
        except Exception as err:
            last_err = err
            time.sleep(0.1)
    if ctx is None:
        raise RuntimeError("failed to connect to LibreOffice UNO: %s" % last_err)

    smgr = ctx.ServiceManager
    desktop = smgr.createInstanceWithContext("com.sun.star.frame.Desktop", ctx)
    doc = desktop.loadComponentFromURL(
        uno.systemPathToFileUrl(pdf_path),
        "_blank",
        0,
        (prop("Hidden", True), prop("ReadOnly", True)),
    )
    try:
        pages = doc.getDrawPages()
        count = pages.getCount()
        exported = []
        for idx in range(min(count, max_pages)):
            page = pages.getByIndex(idx)
            target_path = os.path.join(output_dir, "page-%02d.png" % (idx + 1))
            exporter = smgr.createInstanceWithContext("com.sun.star.drawing.GraphicExportFilter", ctx)
            exporter.setSourceDocument(page)
            ok = exporter.filter((
                prop("URL", uno.systemPathToFileUrl(target_path)),
                prop("MediaType", "image/png"),
            ))
            if ok and os.path.exists(target_path):
                exported.append({
                    "page": idx + 1,
                    "path": target_path,
                    "mime": "image/png",
                })
        with open(manifest_path, "w", encoding="utf-8") as fh:
            json.dump({"pages": exported}, fh, ensure_ascii=False)
    finally:
        doc.close(True)
finally:
    try:
        proc.terminate()
        proc.wait(timeout=5)
    except Exception:
        proc.kill()
    shutil.rmtree(profile_dir, ignore_errors=True)
`
