package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/agent-os/backend/internal/model"
	"github.com/agent-os/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdminMiddlewareRejectsNormalUser(t *testing.T) {
	repo := newAdminTestRepo(t)
	user := &model.User{PubKeyEd25519: "normal", Status: "active", Role: "user"}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	status := exerciseAdminMiddleware(repo, user)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
}

func TestAdminMiddlewareAllowsIsAdminUser(t *testing.T) {
	repo := newAdminTestRepo(t)
	user := &model.User{PubKeyEd25519: "admin", Status: "active", Role: "user", IsAdmin: true}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	status := exerciseAdminMiddleware(repo, user)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
}

func TestAdminMiddlewareAllowsAdminRole(t *testing.T) {
	repo := newAdminTestRepo(t)
	user := &model.User{PubKeyEd25519: "role-admin", Status: "active", Role: "admin"}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	status := exerciseAdminMiddleware(repo, user)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
}

func TestAdminMiddlewareRejectsInactiveAdmin(t *testing.T) {
	repo := newAdminTestRepo(t)
	user := &model.User{PubKeyEd25519: "inactive-admin", Status: "disabled", Role: "admin", IsAdmin: true}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	status := exerciseAdminMiddleware(repo, user)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", status)
	}
}

func newAdminTestRepo(t *testing.T) *repository.UserRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.PathEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repository.NewUserRepo(db)
}

func exerciseAdminMiddleware(repo *repository.UserRepo, user *model.User) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin", func(c *gin.Context) {
		c.Set("user_id", user.ID)
	}, AdminMiddleware(repo), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r.ServeHTTP(rec, req)
	return rec.Code
}
