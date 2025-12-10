package koth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type accessTokenEntry struct {
	competitionID string
	expiresAt     time.Time
}

var (
	accessTokens   = make(map[string]*accessTokenEntry)
	accessTokensMu sync.RWMutex
)

func IssueAccessToken(competitionID string, ttl time.Duration) string {
	var buf = make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}

	var token = hex.EncodeToString(buf)
	accessTokensMu.Lock()
	accessTokens[token] = &accessTokenEntry{
		competitionID: competitionID,
		expiresAt:     time.Now().Add(ttl),
	}
	accessTokensMu.Unlock()

	return token
}

func ValidateAccessToken(competitionID, token string) bool {
	if token == "" {
		return false
	}

	accessTokensMu.RLock()
	entry, ok := accessTokens[token]
	accessTokensMu.RUnlock()
	if !ok {
		return false
	}

	if entry.expiresAt.Before(time.Now()) {
		RevokeAccessToken(token)
		return false
	}

	return entry.competitionID == competitionID
}

func RevokeAccessToken(token string) {
	if token == "" {
		return
	}

	accessTokensMu.Lock()
	delete(accessTokens, token)
	accessTokensMu.Unlock()
}
