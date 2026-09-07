package qrlogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	StatusPending   = "pending"
	StatusScanned   = "scanned"
	StatusConfirmed = "confirmed"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
	StatusConsumed  = "consumed"

	keyPrefix = "qr:login:"
)

var (
	ErrSessionNotFound = errors.New("qr login session not found")
	ErrInvalidStatus   = errors.New("qr login session status invalid")
	ErrScannedByOther  = errors.New("qr login session scanned by another user")
	ErrAlreadyConsumed = errors.New("qr login session already consumed")
	ErrNotConfirmed    = errors.New("qr login session not confirmed")
)

type Session struct {
	Status    string `json:"status"`
	Platform  int32  `json:"platform"`
	DeviceID  string `json:"deviceID"`
	ClientIP  string `json:"clientIP"`
	UserID    string `json:"userID,omitempty"`
	ScannedBy string `json:"scannedBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	ExpireAt  int64  `json:"expireAt"`
}

type Manager struct {
	rdb redis.UniversalClient
	ttl time.Duration
}

func NewManager(rdb redis.UniversalClient, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 120 * time.Second
	}
	return &Manager{rdb: rdb, ttl: ttl}
}

func (m *Manager) key(sessionID string) string {
	return keyPrefix + sessionID
}

func (m *Manager) Create(ctx context.Context, platform int32, deviceID, clientIP string) (*Session, string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	sessionID := hex.EncodeToString(buf)
	now := time.Now()
	session := &Session{
		Status:    StatusPending,
		Platform:  platform,
		DeviceID:  deviceID,
		ClientIP:  clientIP,
		CreatedAt: now.Unix(),
		ExpireAt:  now.Add(m.ttl).Unix(),
	}
	data, err := json.Marshal(session)
	if err != nil {
		return nil, "", err
	}
	if err := m.rdb.Set(ctx, m.key(sessionID), data, m.ttl).Err(); err != nil {
		return nil, "", err
	}
	return session, sessionID, nil
}

func (m *Manager) Get(ctx context.Context, sessionID string) (*Session, error) {
	data, err := m.rdb.Get(ctx, m.key(sessionID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	if session.Status != StatusConsumed && time.Now().Unix() > session.ExpireAt {
		return nil, ErrSessionNotFound
	}
	return &session, nil
}

func (m *Manager) save(ctx context.Context, sessionID string, session *Session) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	ttl := time.Until(time.Unix(session.ExpireAt, 0))
	if ttl <= 0 {
		ttl = time.Second
	}
	return m.rdb.Set(ctx, m.key(sessionID), data, ttl).Err()
}

func (m *Manager) Scan(ctx context.Context, sessionID, userID string) (*Session, error) {
	session, err := m.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	switch session.Status {
	case StatusPending:
		session.Status = StatusScanned
		session.ScannedBy = userID
	case StatusScanned:
		if session.ScannedBy != "" && session.ScannedBy != userID {
			return nil, ErrScannedByOther
		}
		session.ScannedBy = userID
	default:
		return nil, ErrInvalidStatus
	}
	if err := m.save(ctx, sessionID, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (m *Manager) Confirm(ctx context.Context, sessionID, userID string) (*Session, error) {
	session, err := m.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	switch session.Status {
	case StatusPending, StatusScanned:
		if session.ScannedBy != "" && session.ScannedBy != userID {
			return nil, ErrScannedByOther
		}
		session.Status = StatusConfirmed
		session.UserID = userID
	default:
		return nil, ErrInvalidStatus
	}
	if err := m.save(ctx, sessionID, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (m *Manager) Cancel(ctx context.Context, sessionID string) error {
	session, err := m.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == StatusConfirmed || session.Status == StatusConsumed {
		return ErrInvalidStatus
	}
	session.Status = StatusPending
	session.ScannedBy = ""
	return m.save(ctx, sessionID, session)
}

// TryConsumeConfirmed atomically transitions confirmed → consumed.
// Only the caller that succeeds should issue login tokens.
func (m *Manager) TryConsumeConfirmed(ctx context.Context, sessionID string) (*Session, error) {
	key := m.key(sessionID)
	const maxRetry = 3
	for i := 0; i < maxRetry; i++ {
		var consumed *Session
		err := m.rdb.Watch(ctx, func(tx *redis.Tx) error {
			data, err := tx.Get(ctx, key).Bytes()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return ErrSessionNotFound
				}
				return err
			}
			var session Session
			if err := json.Unmarshal(data, &session); err != nil {
				return err
			}
			if session.Status == StatusConsumed {
				return ErrAlreadyConsumed
			}
			if session.Status != StatusConfirmed {
				return ErrNotConfirmed
			}
			if time.Now().Unix() > session.ExpireAt {
				return ErrSessionNotFound
			}
			session.Status = StatusConsumed
			newData, err := json.Marshal(session)
			if err != nil {
				return err
			}
			ttl := time.Until(time.Unix(session.ExpireAt, 0))
			if ttl <= 0 {
				ttl = time.Second
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, newData, ttl)
				return nil
			})
			if err != nil {
				return err
			}
			consumed = &session
			return nil
		}, key)
		if err == nil {
			return consumed, nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return nil, err
	}
	return nil, ErrInvalidStatus
}

// RollbackConsumed restores consumed → confirmed so token issuance can be retried.
func (m *Manager) RollbackConsumed(ctx context.Context, sessionID string) error {
	key := m.key(sessionID)
	const maxRetry = 3
	for i := 0; i < maxRetry; i++ {
		err := m.rdb.Watch(ctx, func(tx *redis.Tx) error {
			data, err := tx.Get(ctx, key).Bytes()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return ErrSessionNotFound
				}
				return err
			}
			var session Session
			if err := json.Unmarshal(data, &session); err != nil {
				return err
			}
			if session.Status != StatusConsumed {
				return nil
			}
			if time.Now().Unix() > session.ExpireAt {
				return ErrSessionNotFound
			}
			session.Status = StatusConfirmed
			newData, err := json.Marshal(session)
			if err != nil {
				return err
			}
			ttl := time.Until(time.Unix(session.ExpireAt, 0))
			if ttl <= 0 {
				ttl = time.Second
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, newData, ttl)
				return nil
			})
			return err
		}, key)
		if err == nil {
			return nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return ErrInvalidStatus
}

func (m *Manager) MarkConsumed(ctx context.Context, sessionID string) error {
	session, err := m.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	session.Status = StatusConsumed
	return m.save(ctx, sessionID, session)
}

func IsDesktopPlatform(platform int32) bool {
	switch platform {
	case 3, 4, 7: // Windows, macOS, Linux
		return true
	default:
		return false
	}
}
