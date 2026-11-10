package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"apps-catalog/internal/store"
)

const (
	maxTries = 5
	lockFor  = 15 * time.Minute

	MinPasswordLen = 8
	// bcrypt ignores anything after 72 bytes
	maxPasswordBytes = 72
)

// lowered in tests
var bcryptCost = 12

var (
	ErrPasswordShort = errors.New("use at least 8 characters")
	ErrPasswordLong  = errors.New("use at most 72 bytes")
)

// HashPassword checks the length and returns a bcrypt hash.
func HashPassword(pw string) (string, error) {
	if len([]rune(pw)) < MinPasswordLen {
		return "", ErrPasswordShort
	}
	if len(pw) > maxPasswordBytes {
		return "", ErrPasswordLong
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// compared against when the username does not exist, so both cases take
// the same time
var dummyHash = sync.OnceValue(func() string {
	h, _ := bcrypt.GenerateFromPassword([]byte("not a real password"), bcryptCost)
	return string(h)
})

type LocalLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *Auth) localLogin(c fiber.Ctx) error {
	var in LocalLogin
	if err := json.Unmarshal(c.Body(), &in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json body")
	}
	username := strings.TrimSpace(in.Username)
	key := strings.ToLower(username) + "|" + c.IP()
	if a.limit.blocked(key) {
		return fiber.NewError(fiber.StatusTooManyRequests, "Too many wrong tries. Wait 15 minutes and try again.")
	}

	subject, hash, err := a.st.LocalCredentials(c.Context(), username)
	if errors.Is(err, store.ErrNotFound) {
		subject, hash = "", dummyHash()
	} else if err != nil {
		return err
	}
	if !CheckPassword(hash, in.Password) || subject == "" {
		a.limit.fail(key)
		a.log.Warn().Str("username", username).Str("ip", c.IP()).Msg("local login failed")
		return fiber.NewError(fiber.StatusUnauthorized, "Wrong username or password.")
	}
	a.limit.reset(key)

	if err := a.st.TouchLogin(c.Context(), subject); err != nil {
		return err
	}
	if err := a.startSession(c, subject); err != nil {
		return err
	}
	a.log.Info().Str("user", username).Msg("local login")
	return c.SendStatus(fiber.StatusNoContent)
}

// limiter blocks a key after max failures until window has passed since
// the first one.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	tries  map[string]tries
}

type tries struct {
	n     int
	first time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, now: time.Now, tries: map[string]tries{}}
}

func (l *limiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t, ok := l.tries[key]
	if !ok {
		return false
	}
	if l.now().Sub(t.first) > l.window {
		delete(l.tries, key)
		return false
	}
	return t.n >= l.max
}

func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.tries) > 10000 {
		for k, t := range l.tries {
			if now.Sub(t.first) > l.window {
				delete(l.tries, k)
			}
		}
	}
	t, ok := l.tries[key]
	if !ok || now.Sub(t.first) > l.window {
		t = tries{first: now}
	}
	t.n++
	l.tries[key] = t
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.tries, key)
}
