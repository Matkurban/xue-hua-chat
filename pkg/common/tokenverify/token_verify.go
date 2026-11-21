package tokenverify

import (
	"errors"
	"time"

	"chat/pkg/common/constant"
	"chat/pkg/common/errs"
	"github.com/golang-jwt/jwt/v5"
)

const (
	TokenUser  = constant.NormalUser
	TokenAdmin = constant.AdminUser
)

type claims struct {
	UserID     string
	UserType   int32
	PlatformID int32
	jwt.RegisteredClaims
}

type Token struct {
	Expires time.Duration
	Secret  string
}

func (t *Token) secret() jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errs.ErrTokenMalformed.WrapMsg("unexpected signing method")
		}
		return []byte(t.Secret), nil
	}
}

func (t *Token) buildClaims(userID string, userType int32) claims {
	now := time.Now()
	return claims{
		UserID:   userID,
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(t.Expires)),    // Expiration time
			IssuedAt:  jwt.NewNumericDate(now),                   // Issuing time
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)), // Begin Effective time
		},
	}
}

func (t *Token) getToken(str string) (string, int32, error) {
	token, err := jwt.ParseWithClaims(
		str,
		&claims{},
		t.secret(),
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenMalformed):
			return "", 0, errs.ErrTokenMalformed.Wrap()
		case errors.Is(err, jwt.ErrTokenExpired):
			return "", 0, errs.ErrTokenExpired.Wrap()
		case errors.Is(err, jwt.ErrTokenNotValidYet):
			return "", 0, errs.ErrTokenNotValidYet.Wrap()
		default:
			return "", 0, errs.ErrTokenUnknown.Wrap()
		}
	}
	c, ok := token.Claims.(*claims)
	if !ok || c == nil || !token.Valid {
		return "", 0, errs.ErrTokenNotValidYet.Wrap()
	}
	if c.PlatformID != 0 {
		return "", 0, errs.ErrTokenExpired.Wrap()
	}
	return c.UserID, c.UserType, nil
}

func (t *Token) CreateToken(UserID string, userType int32) (string, time.Duration, error) {
	if !(userType == TokenUser || userType == TokenAdmin) {
		return "", 0, errs.ErrTokenUnknown.WrapMsg("token type unknown")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, t.buildClaims(UserID, userType))
	str, err := token.SignedString([]byte(t.Secret))
	if err != nil {
		return "", 0, errs.Wrap(err)
	}
	return str, t.Expires, nil
}

func (t *Token) GetToken(token string) (string, int32, error) {
	userID, userType, err := t.getToken(token)
	if err != nil {
		return "", 0, err
	}
	if !(userType == TokenUser || userType == TokenAdmin) {
		return "", 0, errs.ErrTokenUnknown.WrapMsg("token type unknown")
	}
	return userID, userType, nil
}

//func (t *Token) GetAdminTokenCache(token string) (string, error) {
//	userID, userType, err := getToken(token)
//	if err != nil {
//		return "", err
//	}
//	if userType != TokenAdmin {
//		return "", errs.ErrTokenUnknown.WrapMsg("token type error")
//	}
//	return userID, nil
//}
//
//func (t *Token) GetUserToken(token string) (string, error) {
//	userID, userType, err := getToken(token)
//	if err != nil {
//		return "", err
//	}
//	if userType != TokenUser {
//		return "", errs.ErrTokenUnknown.WrapMsg("token type error")
//	}
//	return userID, nil
//}
