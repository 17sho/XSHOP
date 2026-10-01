package middleware

import (
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strings"
)

type callbackRouteSettingsReader interface {
	GetCallbackRoutesCached() *settingsintegration.CallbackRoutesSetting
}

// CustomCallbackRateLimitMiddleware runs before dynamic dispatch. Default routes
// keep their existing route-group limiter and must not be charged twice here.
func CustomCallbackRateLimitMiddleware(settings callbackRouteSettingsReader, client *redis.Client, rule RateLimitRule) gin.HandlerFunc {
	limit := RateLimitMiddleware(client, rule, KeyByIP)
	return func(c *gin.Context) {
		path := strings.TrimRight(c.Request.URL.Path, "/")
		method := c.Request.Method
		if settings != nil && strings.HasPrefix(path, "/api/") && (method == http.MethodGet || method == http.MethodPost) {
			routes := settings.GetCallbackRoutesCached()
			if routes != nil {
				matched := routes.PaymentCallback != "" && path == routes.PaymentCallback
				if method == http.MethodPost {
					for _, target := range []string{routes.DujiaoPayWebhook, routes.PaypalWebhook, routes.StripeWebhook, routes.UpstreamCallback} {
						if target != "" && path == target {
							matched = true
						}
					}
				}
				if matched {
					limit(c)
					return
				}
			}
		}
		c.Next()
	}
}
