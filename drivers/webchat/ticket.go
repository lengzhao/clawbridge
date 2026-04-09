package webchat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type mediaTicketPayload struct {
	Loc  string `json:"loc"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Exp  int64  `json:"exp"`
}

func (d *driver) mediaSecret() []byte {
	sum := sha256.Sum256([]byte("clawbridge.webchat.media.v1\x00" + d.id + "\x00" + d.listen))
	return sum[:]
}

func (d *driver) signMediaTicket(loc, filename, contentType string, ttl time.Duration) (string, error) {
	exp := time.Now().Add(ttl).Unix()
	p := mediaTicketPayload{Loc: loc, Name: filename, Type: contentType, Exp: exp}
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, d.mediaSecret())
	mac.Write(body)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (d *driver) parseMediaTicket(token string) (*mediaTicketPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, errors.New("bad token")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, d.mediaSecret())
	mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, errors.New("bad sig")
	}
	var p mediaTicketPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	if p.Loc == "" || p.Exp < time.Now().Unix() {
		return nil, errors.New("expired or empty loc")
	}
	return &p, nil
}
