package officesdk

import (
	"encoding/json"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type signedTokenPayload struct {
	Signature string `json:"signature"`
}

func SignToken(secret, appID, fileID string) string {
	if secret == "" {
		return fileID
	}
	claims := jwt.MapClaims{
		"fileId": fileID,
		"exp":    time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if appID != "" {
		token.Header["kid"] = appID
	}
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return fileID
	}
	payload, err := json.Marshal(signedTokenPayload{Signature: signed})
	if err != nil {
		return fileID
	}
	return string(payload)
}
