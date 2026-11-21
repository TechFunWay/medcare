package user

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func gatewayMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(MarkFnOSGateway(c.Request.Context()))
		c.Next()
	}
}

// TestIssueSessionCookieOnGateway 网关请求登录成功后种 HttpOnly 会话 Cookie，
// 直连端口请求不种；Cookie 的 Path 必须是网关前缀（与兄弟应用互不干扰）。
func TestIssueSessionCookieOnGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newRouter := func(gateway bool) *gin.Engine {
		r := gin.New()
		handler := func(c *gin.Context) {
			issueSessionCookie(c, nil, "/app/techfunway-test", map[string]interface{}{"token": "jwt-abc"})
			c.Status(http.StatusOK)
		}
		if gateway {
			r.POST("/login", gatewayMiddleware(), handler)
		} else {
			r.POST("/login", handler)
		}
		return r
	}

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	resp := httptest.NewRecorder()
	newRouter(true).ServeHTTP(resp, req)
	cookies := resp.Result().Cookies()
	var session *http.Cookie
	for _, ck := range cookies {
		if ck.Name == sessionCookieName {
			session = ck
		}
	}
	if session == nil {
		t.Fatal("网关请求应种会话 Cookie")
	}
	if session.Value != "jwt-abc" || session.Path != "/app/techfunway-test" || !session.HttpOnly {
		t.Fatalf("Cookie 属性不符: %+v", session)
	}
	if session.MaxAge <= 0 {
		t.Fatalf("MaxAge 应为正值，得到 %d", session.MaxAge)
	}
	if !strings.Contains(strings.ToLower(resp.Header().Get("Set-Cookie")), "samesite=lax") {
		t.Fatalf("应带 SameSite=Lax: %q", resp.Header().Get("Set-Cookie"))
	}

	resp2 := httptest.NewRecorder()
	newRouter(false).ServeHTTP(resp2, httptest.NewRequest(http.MethodPost, "/login", nil))
	if len(resp2.Result().Cookies()) != 0 {
		t.Fatalf("直连端口不应种 Cookie: %v", resp2.Result().Cookies())
	}
}

// TestClearSessionCookie 清除动作对网关与直连端口都生效（Cookie 不分端口）。
func TestClearSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/logout", func(c *gin.Context) {
		clearSessionCookie(c, "/app/techfunway-test")
		c.Status(http.StatusOK)
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/logout", nil))
	cookies := resp.Result().Cookies()
	var session *http.Cookie
	for _, ck := range cookies {
		if ck.Name == sessionCookieName {
			session = ck
		}
	}
	if session == nil {
		t.Fatal("退出应下发清除 Cookie 指令")
	}
	if session.MaxAge >= 0 || session.Value != "" {
		t.Fatalf("清除应为 MaxAge<0 且空值: %+v", session)
	}
	if session.Path != "/app/techfunway-test" {
		t.Fatalf("清除的 Path 必须与种下时一致: %q", session.Path)
	}
}

// TestIsFnOSGatewayRequest 标记只对显式打标的连接生效，TCP 伪造的请求头不算数。
func TestIsFnOSGatewayRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsFnOSGatewayRequest(req) {
		t.Fatal("未打标请求不应识别为网关")
	}
	marked := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(MarkFnOSGateway(req.Context()))
	if !IsFnOSGatewayRequest(marked) {
		t.Fatal("打标请求应识别为网关")
	}
}
