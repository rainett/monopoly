# TODO - Future Improvements

## Critical (Security)

### 1. Add Rate Limiting
- [ ] Install rate limiting middleware (e.g., `golang.org/x/time/rate`)
- [ ] Apply to /api/auth/login (5 attempts per minute per IP)
- [ ] Apply to /api/auth/register (3 attempts per minute per IP)
- [ ] Add exponential backoff for repeated failures

```go
// Example implementation
import "golang.org/x/time/rate"

type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.Mutex
}

func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    limiter, exists := rl.limiters[ip]
    if !exists {
        limiter = rate.NewLimiter(5, 10) // 5 req/sec, burst of 10
        rl.limiters[ip] = limiter
    }
    return limiter
}
```

### 2. Add CSRF Protection
- [ ] Generate CSRF tokens on page load
- [ ] Include token in all state-changing requests
- [ ] Validate token on server side
- [ ] Use `gorilla/csrf` package

```go
import "github.com/gorilla/csrf"

// In server setup
csrfMiddleware := csrf.Protect(
    []byte(cfg.SessionSecret),
    csrf.Secure(false), // Set to true in production with HTTPS
)
```

### 3. Implement Session Expiration
- [ ] Add `ExpiresAt` field to sessions
- [ ] Check expiration on validation
- [ ] Add background cleanup goroutine
- [ ] Set reasonable TTL (e.g., 7 days)

```go
type Session struct {
    UserID    int64
    ExpiresAt time.Time
}

func (sm *SessionManager) CleanupExpired() {
    ticker := time.NewTicker(1 * time.Hour)
    for range ticker.C {
        sm.mu.Lock()
        for id, session := range sm.sessions {
            if time.Now().After(session.ExpiresAt) {
                delete(sm.sessions, id)
            }
        }
        sm.mu.Unlock()
    }
}
```

### 4. Add Input Sanitization
- [ ] Install `bluemonday` for HTML sanitization
- [ ] Sanitize all user inputs (usernames, etc.)
- [ ] Escape output in templates
- [ ] Validate all numeric inputs

```go
import "github.com/microcosm-cc/bluemonday"

var policy = bluemonday.StrictPolicy()

func sanitize(input string) string {
    return policy.Sanitize(input)
}
```

### 5. Strengthen Password Requirements
- [ ] Require 8+ characters
- [ ] Require at least one uppercase letter
- [ ] Require at least one number
- [ ] Require at least one special character
- [ ] Check against common password list

```go
func validatePassword(password string) error {
    if len(password) < 8 {
        return errors.New("password must be at least 8 characters")
    }
    if !regexp.MustCompile(`[A-Z]`).MatchString(password) {
        return errors.New("password must contain uppercase letter")
    }
    if !regexp.MustCompile(`[0-9]`).MatchString(password) {
        return errors.New("password must contain number")
    }
    if !regexp.MustCompile(`[!@#$%^&*]`).MatchString(password) {
        return errors.New("password must contain special character")
    }
    return nil
}
```

## High Priority (Production Readiness)

### 6. Configuration Management
- [ ] Create config file support (YAML/JSON)
- [ ] Use environment variables
- [ ] Add config validation
- [ ] Support different environments (dev/staging/prod)

```go
type Config struct {
    ServerPort    string `env:"SERVER_PORT" envDefault:":8080"`
    DBPath        string `env:"DB_PATH" envDefault:"./monopoly.db"`
    SessionSecret string `env:"SESSION_SECRET,required"`
    Environment   string `env:"ENVIRONMENT" envDefault:"development"`
}

// Use github.com/caarlos0/env for loading
```

### 7. Structured Logging
- [ ] Replace log package with structured logger (zap/zerolog)
- [ ] Add log levels (debug, info, warn, error)
- [ ] Add request IDs for tracing
- [ ] Log to file in production

```go
import "go.uber.org/zap"

logger, _ := zap.NewProduction()
defer logger.Sync()

logger.Info("server starting",
    zap.String("port", cfg.ServerPort),
    zap.String("environment", cfg.Environment),
)
```

### 8. Database Connection Pooling
- [ ] Configure max connections
- [ ] Add connection timeout
- [ ] Add query timeout
- [ ] Monitor connection health

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(10 * time.Minute)
```

### 9. Health Check Endpoint
- [ ] Add /health endpoint
- [ ] Check database connectivity
- [ ] Check memory usage
- [ ] Return service status

```go
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
    if err := h.store.Ping(); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(map[string]string{
            "status": "unhealthy",
            "error":  err.Error(),
        })
        return
    }

    json.NewEncoder(w).Encode(map[string]string{
        "status": "healthy",
    })
}
```

### 10. Database Migrations
- [ ] Use migration tool (golang-migrate or goose)
- [ ] Version all schema changes
- [ ] Add rollback support
- [ ] Automate migrations in CI/CD

```bash
# Using golang-migrate
migrate -database sqlite3://monopoly.db -path migrations up
```

## Medium Priority (Code Quality)

### 11. Add Unit Tests
- [ ] Test auth service
- [ ] Test game engine state transitions
- [ ] Test WebSocket message handling
- [ ] Test API handlers
- [ ] Aim for 80%+ coverage

```go
func TestLogin(t *testing.T) {
    mockStore := &MockStore{}
    sessionManager := auth.NewSessionManager("secret")
    service := auth.NewService(mockStore, sessionManager)

    // Test cases...
}
```

### 12. Add Integration Tests
- [ ] Test full user registration flow
- [ ] Test game creation and joining
- [ ] Test turn management
- [ ] Test WebSocket communication

### 13. Use Prepared Statements
- [ ] Convert all queries to prepared statements
- [ ] Cache prepared statements
- [ ] Measure performance improvement

```go
type SQLiteStore struct {
    db              *sql.DB
    createUserStmt  *sql.Stmt
    getUserStmt     *sql.Stmt
    // ... other statements
}

func (s *SQLiteStore) prepareStatements() error {
    var err error
    s.createUserStmt, err = s.db.Prepare(
        "INSERT INTO users (username, password_hash) VALUES (?, ?)",
    )
    return err
}
```

### 14. Refactor Duplicate Code
- [ ] Extract GetGameState to shared helper
- [ ] Create game state builder
- [ ] Extract player conversion logic

```go
// Shared helper
func buildGameState(game *Game, players []*GamePlayer) *GameState {
    // Common logic
}
```

### 15. Add Request Validation Middleware
- [ ] Validate Content-Type headers
- [ ] Validate request body size
- [ ] Validate JSON structure
- [ ] Return clear validation errors

## Low Priority (Nice to Have)

### 16. Add Metrics
- [ ] Use Prometheus metrics
- [ ] Track request counts
- [ ] Track response times
- [ ] Track error rates
- [ ] Add /metrics endpoint

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )
)
```

### 17. Add Graceful WebSocket Shutdown
- [ ] Track active WebSocket connections
- [ ] Send close messages on shutdown
- [ ] Wait for connections to close
- [ ] Add timeout for forced closure

### 18. Add Database Backups
- [ ] Automated backup script
- [ ] Store backups off-site
- [ ] Test restore procedure
- [ ] Schedule regular backups

### 19. Add API Documentation
- [ ] Use OpenAPI/Swagger
- [ ] Document all endpoints
- [ ] Include request/response examples
- [ ] Host interactive docs

### 20. Add Frontend Build Pipeline
- [ ] Use bundler (Vite/Webpack)
- [ ] Minify JavaScript
- [ ] Optimize CSS
- [ ] Add source maps
- [ ] Hash filenames for caching

## Performance Optimizations

### 21. Add Caching
- [ ] Cache game list (Redis/in-memory)
- [ ] Cache game state
- [ ] Invalidate on updates
- [ ] Add cache headers

### 22. Add Database Indexes
- [ ] Index on users.username (already has UNIQUE)
- [ ] Index on games.status (already exists)
- [ ] Index on game_players.is_current_turn
- [ ] Analyze slow queries

### 23. Optimize Frontend Polling
- [ ] Use WebSocket for lobby updates
- [ ] Reduce poll frequency
- [ ] Only poll when tab is active
- [ ] Add exponential backoff on errors

```javascript
let pollInterval = 3000;
let errorCount = 0;

async function loadGames() {
    try {
        const games = await api.listGames();
        displayGames(games);
        errorCount = 0;
        pollInterval = 3000;
    } catch (error) {
        errorCount++;
        pollInterval = Math.min(30000, pollInterval * 2);
    }
    setTimeout(loadGames, pollInterval);
}
```

## DevOps

### 24. Add Docker Support
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o monopoly .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/monopoly .
COPY --from=builder /app/static ./static
EXPOSE 8080
CMD ["./monopoly"]
```

### 25. Add CI/CD Pipeline
- [ ] GitHub Actions workflow
- [ ] Run tests on PR
- [ ] Build and push Docker image
- [ ] Deploy to staging/production

### 26. Add Monitoring
- [ ] Set up error tracking (Sentry)
- [ ] Set up APM (Datadog/New Relic)
- [ ] Set up uptime monitoring
- [ ] Configure alerts

## Estimated Effort

- **Critical (1-5)**: 2-3 days
- **High Priority (6-10)**: 3-4 days
- **Medium Priority (11-15)**: 3-4 days
- **Low Priority (16-20)**: 2-3 days
- **Performance (21-23)**: 1-2 days
- **DevOps (24-26)**: 2-3 days

**Total**: ~13-19 days for full production readiness
