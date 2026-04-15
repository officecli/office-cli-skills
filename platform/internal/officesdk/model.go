package officesdk

type FileMeta struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	StorageKey string `json:"storage_key,omitempty"`
	Version    uint32 `json:"version"`
	FromSDK    bool   `json:"from_sdk"`
	CreateTime int64  `json:"create_time"`
	ModifyTime int64  `json:"modify_time"`
	CreatorID  string `json:"creator_id"`
	ModifierID string `json:"modifier_id"`
	AppID      string `json:"app_id,omitempty"`
}

type UploadInfo struct {
	PresignedPutURL string `json:"presigned_put_url"`
	ContentType     string `json:"content_type"`
}

type SDKParams struct {
	Endpoint string `json:"endpoint"`
	Path     string `json:"path,omitempty"`
	Token    string `json:"token,omitempty"`
	TokenURL string `json:"tokenUrl,omitempty"`
	FileID   string `json:"fileId"`
	FileType int    `json:"fileType"`
	Version  uint32 `json:"version"`
}
