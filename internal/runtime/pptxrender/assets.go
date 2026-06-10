package pptxrender

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

type AssetResolver interface {
	Resolve(ctx context.Context, id string) (AssetData, error)
}

type AssetData struct {
	Bytes       []byte
	Ext         string
	ContentType string
	AltText     string
}

type mapAssetResolver struct {
	assets map[string]AssetData
}

func NewMapAssetResolver(assets map[string]AssetData) AssetResolver {
	copied := make(map[string]AssetData, len(assets))
	for id, asset := range assets {
		copied[id] = normalizeAssetData(asset, id)
	}
	return &mapAssetResolver{assets: copied}
}

func (r *mapAssetResolver) Resolve(ctx context.Context, id string) (AssetData, error) {
	if err := ctx.Err(); err != nil {
		return AssetData{}, err
	}
	asset, ok := r.assets[id]
	if !ok {
		return AssetData{}, fmt.Errorf("unknown asset %q", id)
	}
	return asset, nil
}

type fileAssetResolver struct {
	baseDir string
	assets  map[string]Asset
}

func NewFileAssetResolver(baseDir string, assetLists ...[]Asset) AssetResolver {
	resolver := &fileAssetResolver{
		baseDir: baseDir,
		assets:  make(map[string]Asset),
	}
	for _, assetList := range assetLists {
		for _, asset := range assetList {
			resolver.assets[asset.ID] = asset
		}
	}
	return resolver
}

func (r *fileAssetResolver) Resolve(ctx context.Context, id string) (AssetData, error) {
	if err := ctx.Err(); err != nil {
		return AssetData{}, err
	}

	asset, ok := r.assets[id]
	path := id
	contentType := ""
	altText := ""
	if ok {
		path = asset.Path
		contentType = asset.ContentType
		altText = asset.Alt
	}
	if path == "" {
		return AssetData{}, fmt.Errorf("asset %q has empty path", id)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return AssetData{}, err
	}
	return normalizeAssetData(AssetData{
		Bytes:       data,
		Ext:         strings.TrimPrefix(filepath.Ext(path), "."),
		ContentType: contentType,
		AltText:     altText,
	}, id), nil
}

func normalizeAssetData(asset AssetData, id string) AssetData {
	asset.Ext = strings.ToLower(strings.TrimPrefix(asset.Ext, "."))
	if asset.Ext == "" {
		asset.Ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(id), "."))
	}
	if asset.ContentType == "" && asset.Ext != "" {
		asset.ContentType = mime.TypeByExtension("." + asset.Ext)
	}
	if asset.ContentType == "" {
		asset.ContentType = "image/png"
	}
	if asset.Ext == "" {
		asset.Ext = extensionForContentType(asset.ContentType)
	}
	if asset.Ext == "" {
		asset.Ext = "png"
	}
	return asset
}

func extensionForContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/svg+xml":
		return "svg"
	default:
		return ""
	}
}
