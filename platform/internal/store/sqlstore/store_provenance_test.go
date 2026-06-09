package sqlstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/model"
)

func TestCreateImageGenerationProvenanceDuplicateRequestIDLoadsExisting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:image_generation_provenance_duplicate?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ImageGenerationProvenance{}))
	store := NewWithDB(db)

	first := &model.ImageGenerationProvenance{
		RequestID:            "req-provenance",
		UserID:               42,
		Prompt:               "prompt",
		PromptSHA256:         "prompt-hash",
		ImageSHA256:          "first-image-hash",
		ImageStorageKey:      "image-generations/42/first.png",
		ImageContentType:     "image/png",
		ImageOriginalName:    "first.png",
		SourceTemplatePrompt: "",
	}
	require.NoError(t, store.CreateImageGenerationProvenance(context.Background(), first))
	require.NotZero(t, first.ID)

	duplicate := &model.ImageGenerationProvenance{
		RequestID:         "req-provenance",
		UserID:            42,
		Prompt:            "prompt",
		PromptSHA256:      "prompt-hash",
		ImageSHA256:       "second-image-hash",
		ImageStorageKey:   "image-generations/42/second.png",
		ImageContentType:  "image/png",
		ImageOriginalName: "second.png",
	}
	require.NoError(t, store.CreateImageGenerationProvenance(context.Background(), duplicate))
	require.Equal(t, first.ID, duplicate.ID)
	require.Equal(t, "first-image-hash", duplicate.ImageSHA256)
	require.Equal(t, "image-generations/42/first.png", duplicate.ImageStorageKey)
}
