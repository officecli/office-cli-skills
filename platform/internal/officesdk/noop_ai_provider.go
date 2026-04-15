package officesdk

import (
	"github.com/gin-gonic/gin"
	sdkoffice "github.com/officesdk/go-sdk/officesdk"
)

type NoopAIProvider struct{}

func NewNoopAIProvider() *NoopAIProvider { return &NoopAIProvider{} }

func (p *NoopAIProvider) AIConfig(_ *gin.Context) (*sdkoffice.AIConfigResponse, error) {
	return &sdkoffice.AIConfigResponse{LLMList: []sdkoffice.LLMConfig{}}, nil
}

func (p *NoopAIProvider) NewConversation(_ *gin.Context) error { return nil }

func (p *NoopAIProvider) AddMessage(_ *gin.Context, _ string) error { return nil }

func (p *NoopAIProvider) GetConversation(_ *gin.Context, conversationID string) (*sdkoffice.ChatConversation, error) {
	return &sdkoffice.ChatConversation{
		ConversationId: conversationID,
		Messages:       []sdkoffice.ChatMessageDO{},
	}, nil
}

func (p *NoopAIProvider) DeleteConversation(_ *gin.Context, _ string) error { return nil }

func (p *NoopAIProvider) GetFileConversations(_ *gin.Context, _ string) ([]sdkoffice.ChatConversation, error) {
	return []sdkoffice.ChatConversation{}, nil
}

func (p *NoopAIProvider) DeleteFileConversations(_ *gin.Context, _ string) error { return nil }

func (p *NoopAIProvider) BreakConversation(_ *gin.Context, _ string) error { return nil }

func (p *NoopAIProvider) IsConversationBreak(_ *gin.Context, _ string) (*sdkoffice.IsBrokenResponse, error) {
	return &sdkoffice.IsBrokenResponse{Broken: false}, nil
}

func (p *NoopAIProvider) ResumeConversation(_ *gin.Context, _ string) error { return nil }

func (p *NoopAIProvider) DeleteExpireKeys(_ *gin.Context) error { return nil }
