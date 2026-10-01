package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// SensitiveResponseCacheMiddleware protects order/fulfillment/card responses,
// including downloads and authentication failures, without changing public
// catalog caching. Mount on the API group before authentication/authorization.
// Match registered route families (with a segment boundary), not raw query data.
func SensitiveResponseCacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		route := c.FullPath()
		for _, prefix := range sensitiveResponseRouteFamilies {
			if route == prefix || strings.HasPrefix(route, prefix+"/") {
				// Set before c.Next: streamed responses commit headers during the handler.
				c.Header("Cache-Control", "private, no-store")
				break
			}
		}
		c.Next()
	}
}

var sensitiveResponseRouteFamilies = []string{
	"/api/v1/orders",
	"/api/v1/guest/orders",
	"/api/v1/reseller/orders",
	"/api/v1/channel/orders",
	"/api/v1/channel/payments",
	"/api/v1/upstream/orders",
	"/api/v1/admin/orders",
	"/api/v1/admin/order-refunds",
	"/api/v1/admin/fulfillments",
	"/api/v1/admin/card-secrets",
	"/api/v1/admin/gift-cards",
	"/api/v1/admin/procurement-orders",
	"/api/v1/gift-cards",
	"/api/v1/channel/wallet/gift-card",
	"/api/v1/payments",
	"/api/v1/guest/payments",
	"/api/v1/admin/payments",
}
