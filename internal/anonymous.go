package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type AnonymousAuthResponse struct {
	Token string `json:"token"`
}

type anonymousTokenState struct {
	mu         sync.Mutex
	cond       *sync.Cond
	token      string
	expireAt   time.Time
	refreshing bool
}

func newAnonymousTokenState() *anonymousTokenState {
	state := &anonymousTokenState{}
	state.cond = sync.NewCond(&state.mu)
	return state
}

var cachedAnonymousToken = newAnonymousTokenState()

// GetAnonymousToken 从 z.ai 获取匿名 token
func GetAnonymousToken() (string, error) {
	now := time.Now()
	cachedAnonymousToken.mu.Lock()
	if token, ok := cachedAnonymousToken.getValidTokenLocked(now); ok {
		cachedAnonymousToken.mu.Unlock()
		return token, nil
	}

	for cachedAnonymousToken.refreshing {
		cachedAnonymousToken.cond.Wait()
		if token, ok := cachedAnonymousToken.getValidTokenLocked(time.Now()); ok {
			cachedAnonymousToken.mu.Unlock()
			return token, nil
		}
	}

	cachedAnonymousToken.refreshing = true
	cachedAnonymousToken.mu.Unlock()

	token, expireAt, err := fetchAnonymousToken()

	cachedAnonymousToken.mu.Lock()
	defer cachedAnonymousToken.mu.Unlock()
	cachedAnonymousToken.refreshing = false
	cachedAnonymousToken.cond.Broadcast()
	if err != nil {
		return "", err
	}

	cachedAnonymousToken.token = token
	cachedAnonymousToken.expireAt = expireAt
	return token, nil
}

func (s *anonymousTokenState) getValidTokenLocked(now time.Time) (string, bool) {
	if s.token == "" || s.expireAt.IsZero() {
		return "", false
	}

	// 预留安全窗口，避免 token 在请求过程中刚好过期。
	if now.Before(s.expireAt.Add(-30 * time.Second)) {
		return s.token, true
	}
	return "", false
}

func fetchAnonymousToken() (string, time.Time, error) {
	client := GetRandomProxyClient()
	resp, err := client.Get("https://chat.z.ai/api/v1/auths/")
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	var authResp AnonymousAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return "", time.Time{}, err
	}
	if authResp.Token == "" {
		return "", time.Time{}, fmt.Errorf("empty anonymous token")
	}

	expireAt := time.Now().Add(8 * time.Minute)
	if payload, err := DecodeJWTPayload(authResp.Token); err == nil && payload != nil && payload.Exp > 0 {
		expireAt = time.Unix(payload.Exp, 0)
	}

	return authResp.Token, expireAt, nil
}
