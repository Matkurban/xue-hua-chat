package tokenverify

import (
	"testing"
	"time"

	"chat/pkg/common/errs"
	"github.com/golang-jwt/jwt/v5"
)

func TestTokenCreateAndGet(t *testing.T) {
	tv := &Token{
		Expires: time.Hour,
		Secret:  "chat-test-secret",
	}

	tok, exp, err := tv.CreateToken("u100", TokenUser)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if exp != time.Hour {
		t.Fatalf("unexpected expires: %v", exp)
	}

	uid, utype, err := tv.GetToken(tok)
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	if uid != "u100" || utype != TokenUser {
		t.Fatalf("unexpected result: uid=%s utype=%d", uid, utype)
	}
}

func TestTokenErrors(t *testing.T) {
	tv := &Token{
		Expires: time.Hour,
		Secret:  "chat-test-secret",
	}

	if _, _, err := tv.GetToken("bad.token"); !errs.ErrTokenMalformed.Is(err) {
		t.Fatalf("expected ErrTokenMalformed, got: %v", err)
	}

	expired := &Token{Expires: -time.Hour, Secret: tv.Secret}
	expiredTok, _, err := expired.CreateToken("u100", TokenAdmin)
	if err != nil {
		t.Fatalf("CreateToken failed: %v", err)
	}
	if _, _, err := tv.GetToken(expiredTok); !errs.ErrTokenExpired.Is(err) {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}

	hs512Tok, err := jwt.NewWithClaims(jwt.SigningMethodHS512, tv.buildClaims("u100", TokenUser)).SignedString([]byte(tv.Secret))
	if err != nil {
		t.Fatalf("sign HS512 token failed: %v", err)
	}
	if _, _, err := tv.GetToken(hs512Tok); err == nil {
		t.Fatalf("expected HS512 token to be rejected")
	}
}
