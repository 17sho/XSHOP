package publicconfigwiring

import (
	"fmt"
	"github.com/dujiao-next/internal/app/container"
	"github.com/gin-gonic/gin"
)

// ResolveSiteIcon shares the canonical settings and tenant overlay, without a second cache.
func ResolveSiteIcon(c *container.Container) func(*gin.Context) (string, error) {
	return func(ctx *gin.Context) (string, error) {
		tenant, err := c.ResellerDomainResolver.ResolveRequest(ctx.Request.Context(), ctx.Request)
		if err != nil {
			return "", err
		}
		data, err := c.SettingService.GetConfig(map[string]interface{}{})
		if err != nil {
			return "", err
		}
		if tenant.ResellerID != nil {
			if c.ResellerSiteConfigService == nil {
				return "", fmt.Errorf("tenant branding unavailable")
			}
			data, err = c.ResellerSiteConfigService.ApplyPublicConfigOverlay(ctx.Request.Context(), tenant, data)
			if err != nil {
				return "", err
			}
		}
		if brand, ok := data["brand"].(map[string]interface{}); ok {
			if value, ok := brand["site_icon"].(string); ok {
				return value, nil
			}
		}
		return "", nil
	}
}
