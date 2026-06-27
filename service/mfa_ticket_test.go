package service

import (
	"errors"
	"testing"
	"time"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/cache"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
)

// setupTicketTest 为单元测试准备最小化全局环境:
//   - 写入测试 JWT key(签名密钥来源)。
//   - 用内存 cache 提供 nonce 抗重放支撑。
//   - 注入 Config.Mfa,默认 TTL=2s,bindIP=true,maxAttempts=3。
func setupTicketTest(t *testing.T, bindIP bool, ttl time.Duration) *MfaTicketService {
	t.Helper()
	if ttl <= 0 {
		ttl = 2 * time.Second
	}
	global.Jwt = jwt.NewJwt("mfa-ticket-test-key-very-secret", time.Hour)
	global.Cache = cache.New(cache.TypeMem)

	cfg := &config.Config{}
	cfg.Mfa.TicketTTL = ttl
	cfg.Mfa.TicketBindIP = bindIP
	cfg.Mfa.LoginMfaMaxAttempts = 3
	Config = cfg
	Jwt = global.Jwt

	return &MfaTicketService{}
}

func TestIssueAndVerify_OK(t *testing.T) {
	s := setupTicketTest(t, true, 60*time.Second)
	tok, jti, err := s.Issue(42, "1.2.3.4", "device-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if tok == "" || jti == "" {
		t.Fatalf("expected non-empty token+jti, got token=%q jti=%q", tok, jti)
	}
	claims, err := s.Verify(tok, "1.2.3.4")
	if err != nil {
		t.Fatalf("verify ok case: %v", err)
	}
	if claims.UID != 42 {
		t.Fatalf("uid mismatch: want 42, got %d", claims.UID)
	}
	if claims.JTI != jti {
		t.Fatalf("jti mismatch: want %s got %s", jti, claims.JTI)
	}
}

func TestVerify_Replay(t *testing.T) {
	s := setupTicketTest(t, true, 60*time.Second)
	tok, _, err := s.Issue(7, "9.9.9.9", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := s.Verify(tok, "9.9.9.9")
	if err != nil {
		t.Fatalf("first verify: %v", err)
	}
	s.Consume(claims)
	_, err = s.Verify(tok, "9.9.9.9")
	if !errors.Is(err, ErrTicketConsumed) {
		t.Fatalf("expected ErrTicketConsumed on replay, got %v", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	s := setupTicketTest(t, true, 1*time.Second)
	tok, _, err := s.Issue(11, "5.5.5.5", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	_, err = s.Verify(tok, "5.5.5.5")
	if !errors.Is(err, ErrTicketExpired) {
		t.Fatalf("expected ErrTicketExpired, got %v", err)
	}
}

func TestVerify_IPMismatch(t *testing.T) {
	s := setupTicketTest(t, true, 60*time.Second)
	tok, _, err := s.Issue(1, "1.1.1.1", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	_, err = s.Verify(tok, "2.2.2.2")
	if !errors.Is(err, ErrTicketIPMismatch) {
		t.Fatalf("expected ErrTicketIPMismatch, got %v", err)
	}
}

func TestVerify_IPMismatch_DisabledByConfig(t *testing.T) {
	s := setupTicketTest(t, false, 60*time.Second)
	tok, _, err := s.Issue(1, "1.1.1.1", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := s.Verify(tok, "8.8.8.8")
	if err != nil {
		t.Fatalf("expected verify ok when bindIP disabled, got %v", err)
	}
	if claims.UID != 1 {
		t.Fatalf("uid mismatch: %d", claims.UID)
	}
}

func TestVerify_TamperedSignature(t *testing.T) {
	s := setupTicketTest(t, true, 60*time.Second)
	tok, _, err := s.Issue(3, "1.1.1.1", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	// 翻转最后一个字符,模拟签名/payload 篡改。
	if len(tok) == 0 {
		t.Fatalf("empty token")
	}
	bad := tok[:len(tok)-1] + "x"
	if _, err := s.Verify(bad, "1.1.1.1"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("expected ErrTicketInvalid for tampered token, got %v", err)
	}
}

func TestIncAttempt_ExceedsThreshold(t *testing.T) {
	s := setupTicketTest(t, true, 60*time.Second)
	tok, _, err := s.Issue(9, "1.1.1.1", "")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := s.Verify(tok, "1.1.1.1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	// maxAttempts=3 → 第 3 次返回 exceed=true。
	for i := 1; i <= 3; i++ {
		cur, exceed := s.IncAttempt(claims)
		if cur != i {
			t.Fatalf("attempt %d: got count %d", i, cur)
		}
		if i < 3 && exceed {
			t.Fatalf("attempt %d should not exceed yet", i)
		}
		if i == 3 && !exceed {
			t.Fatalf("attempt %d should exceed threshold", i)
		}
	}
}
