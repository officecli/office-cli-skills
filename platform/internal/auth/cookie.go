package auth

import "github.com/gorilla/securecookie"

type SecureCookieCodec struct{ codec *securecookie.SecureCookie }

func NewSecureCookieCodec(secret string) *SecureCookieCodec {
	hashKey := []byte(secret)
	blockKey := []byte(secret)
	if len(blockKey) > 32 {
		blockKey = blockKey[:32]
	}
	return &SecureCookieCodec{codec: securecookie.New(hashKey, blockKey)}
}

func (c *SecureCookieCodec) Encode(sessionID string) (string, error) {
	return c.codec.Encode("app_session", map[string]string{"sid": sessionID})
}

func (c *SecureCookieCodec) Decode(value string) (string, error) {
	payload := map[string]string{}
	if err := c.codec.Decode("app_session", value, &payload); err != nil {
		return "", err
	}
	return payload["sid"], nil
}
