package server

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/shopicano/shopicano-backend/api"
	"net/http"
)

var router = echo.New()

// GetRouter returns the api router
func GetRouter() http.Handler {
	router.Use(middleware.Logger())
	router.Use(middleware.Recover())
	router.Pre(middleware.AddTrailingSlash())
	router.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"*"},
		AllowMethods: []string{"*"},
	}))

	router.GET("/", func(ctx echo.Context) error {
		return ctx.String(http.StatusOK, "OK")
	})

	registerV1Routes()

	return router
}

func registerV1Routes() {
	v1 := router.Group("/v1")

	platformGroup := v1.Group("/platform")
	userGroup := v1.Group("/users")

	storeGroup := v1.Group("/stores")
	categoryGroup := v1.Group("/categories")
	collectionGroup := v1.Group("/collections")
	productGroup := v1.Group("/products")
	couponGroup := v1.Group("/coupons")
	addressesGroup := v1.Group("/addresses")
	ordersGroup := v1.Group("/orders")
	paymentGroup := v1.Group("/payments")
	customersGroup := v1.Group("/customers")
	statsGroup := v1.Group("/stats")
	locationGroup := v1.Group("/locations")

	fsGroup := v1.Group("/fs")

	api.RegisterLegacyRoutes(v1)
	api.RegisterPlatformRoutes(platformGroup)
	api.RegisterUserRoutes(userGroup)
	api.RegisterStoreRoutes(storeGroup)
	api.RegisterProductRoutes(productGroup)
	api.RegisterCategoryRoutes(categoryGroup)
	api.RegisterCollectionRoutes(collectionGroup)
	api.RegisterFSRoutes(fsGroup)
	api.RegisterAddressRoutes(addressesGroup)
	api.RegisterOrderRoutes(ordersGroup)
	api.RegisterPaymentRoutes(paymentGroup)
	api.RegisterCustomerRoutes(customersGroup)
	api.RegisterStatsRoutes(statsGroup)
	api.RegisterCouponRoutes(couponGroup)
	api.RegisterLocationRoutes(locationGroup)
}
