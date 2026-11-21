package user

// 网关域的会话 Cookie 通道（2026-10-02 家族定调，rental f0df3f0 为参考实现）：
// 飞牛统一网关会拦截应用自身的 Authorization 头，SPA 留在网关域名下时应用 JWT
// 没有传输通道。因此登录/注册成功且请求走网关 socket 时，服务端把令牌同时种成
// HttpOnly 会话 Cookie；authenticate 里 Cookie 与 Authorization 同级，网关域上
// 靠它维持登录态。Cookie 不分端口，直连端口上的退出也要清掉。
//
// 硬规矩（记忆 fnos-login-explicit-only）：账密登录/注册绝不读取网关注入身份，
// 这里只是给应用自己的会话补传输通道，不做任何隐式认人。

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gorm.io/gorm"
)

const sessionCookieName = "token"

// sessionCookieMaxAge 跟随登录令牌 TTL（login_expiry_days）；db 为 nil 时
// （单元测试）退回默认的 7 天。
func sessionCookieMaxAge(db *gorm.DB) int {
	if db == nil {
		return 7 * 24 * 3600
	}
	return int(loginTokenTTL(db).Seconds())
}

func setSessionCookie(c *gin.Context, prefix, token string, maxAge int) {
	if prefix == "" || token == "" {
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, token, maxAge, prefix, "", false, true)
}

func clearSessionCookie(c *gin.Context, prefix string) {
	if prefix == "" {
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, prefix, "", false, true)
}

// issueSessionCookie 在网关请求上为刚签发的令牌种会话 Cookie；
// 直连端口请求维持纯 JWT（Authorization 头），不种。
func issueSessionCookie(c *gin.Context, db *gorm.DB, prefix string, result map[string]interface{}) {
	if !IsFnOSGatewayRequest(c.Request) {
		return
	}
	token, _ := result["token"].(string)
	if token == "" {
		return
	}
	setSessionCookie(c, prefix, token, sessionCookieMaxAge(db))
}
