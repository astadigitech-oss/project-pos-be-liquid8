package api

import (
	"liquid8/pos/http/controllers"
	"liquid8/pos/http/middleware"

	"github.com/gin-gonic/gin"
)

func roleGroup(g *gin.RouterGroup, roles []string, fn func(rg *gin.RouterGroup)) {
    rg := g.Group("")
    rg.Use(middleware.RoleCheck(roles))
    fn(rg)
}



func RouteHandler(r *gin.Engine) {
	api := r.Group("/api") 

	// Route public
	//========================================
	// AUTH
	//========================================
	api.GET("/checkLogin", controllers.CheckToken) //AuthController.go
	api.POST("/login", controllers.Login) //AuthController.go
	api.POST("/logout", controllers.Logout) //AuthController.go
	api.POST("/oauth/token", controllers.OAuthServiceAPI) //AuthController.go
	// // with rolecheck example
	// adminOnly := protected.Group("").Use(middleware.RoleCheck([]string{"Admin"}))
	// {
	// 	adminOnly.POST("/generate", controllers.ProcessExcelHandler)
	// 	adminOnly.POST("/generate/merge-headers", controllers.MapAndMergeHeaders)
	// }

	// Route protected
	protected := api.Group("")
	protected.Use(middleware.AuthCheck())
	{

		/*======================= KASIR ONLY =======================*/
		roleGroup(protected, []string{"kasir"}, func(rg *gin.RouterGroup) {
	
			//========================================
			// SHIFT
			//========================================
			rg.GET("shifts", controllers.GetShiftsByCashier) //ShiftController.go
			rg.GET("shifts-active", controllers.CurrentShift) //ShiftController.go
			rg.GET("shifts/:shift_id/transaction", controllers.DetailTransactionsShift) //TransactionController.go
			rg.GET("shifts/all", controllers.GetAllShifts) //ShiftController.go
			rg.POST("shifts/start", controllers.StartShift) //ShiftController.go
			rg.POST("shifts/end", controllers.EndShift)	//ShiftController.go
			
			//========================================
			// PRODUCT
			//========================================
			rg.GET("products-by-store", controllers.ListProductsOfStore) //ProductController.go

			//========================================
			// TRANSACTION
			//========================================
			// Cart / Keranjang
			rg.GET("carts/current", middleware.ShiftCheck(), controllers.GetCurrentCart) //TransactionController.go
			rg.GET("carts/pending", middleware.ShiftCheck(), controllers.ListPending) //TransactionController.go
			rg.PUT("carts/:keep_code/resume-check", middleware.ShiftCheck(), controllers.ResumePendingCheck) //TransactionController.go
			rg.POST("carts/item", middleware.ShiftCheck(), controllers.AddToCart) //TransactionController.go
			rg.POST("carts/pending", middleware.ShiftCheck(), controllers.PendingCart) //TransactionController.go
			rg.DELETE("carts/:cart_id", middleware.ShiftCheck(), controllers.RemoveItemCart) //TransactionController.go
			rg.DELETE("carts/pending/:keep_code", middleware.ShiftCheck(), controllers.RemoveCartByKeepCode) //TransactionController.go
			rg.DELETE("carts/current", controllers.EmptyCurrentCart) //TransactionController.go

			// Transaction
			rg.GET("transactions", controllers.GetTransactionHistories) //TransactionController.go
			rg.GET("transactions/:id", controllers.DetailTransaction) //TransactionController.go
			rg.POST("transactions/checkout", middleware.ShiftCheck(), controllers.CheckoutTransaction) //TransactionController.go
			rg.DELETE("transactions/:id", controllers.CancelTransaction) //TransactionController.go

		})
	
	/*======================= ALL ROLE =======================*/
		//========================================
		// MEMBER
		//========================================
		protected.GET("members", controllers.ListAllMembers) //MemberController.go
		protected.GET("members/:id", controllers.DetailMember) //MemberController.go
		protected.POST("members", controllers.CreateMember) //MemberController.go
		protected.PUT("members/:id", controllers.UpdateMember) //MemberController.go
		protected.DELETE("members/:id", controllers.DeleteMember) //MemberController.go
		//========================================
		// USER
		//========================================
		protected.GET("users-info", controllers.UserInfo) //UserController.go
		protected.PUT("users-profile", controllers.UpdateProfile) //UserController.go
		protected.PUT("users-password", controllers.ChangePassword) //UserController.go

	/*======================= ADMIN ONLY =======================*/
		roleGroup(protected, []string{"superadmin","admin"}, func(rg *gin.RouterGroup) {
			//========================================
			// PPN
			//========================================
			rg.GET("ppns", controllers.GetPPN) //PPNController.go
			rg.GET("ppns/:id", controllers.DetailPPN) //PPNController.go
			rg.POST("ppns", controllers.StorePPN) //PPNController.go
			rg.PUT("ppns/:id", controllers.UpdatePPN) //PPNController.go
			rg.DELETE("ppns/:id", controllers.DeletePPN) //PPNController.go
			
			//========================================
			// DASHBOARD
			//========================================
			//statistik dashboard
			rg.GET("dashboard/index", controllers.GetDashboardData) //DashboardController.go
			rg.GET("dashboard/sales-total", controllers.GetTotalSalesByFilter) //DashboardController.go
			
			//========================================
			// TOKO
			//========================================
			rg.GET("stores", controllers.ListStores) //StoreController.go
			rg.GET("stores/:id", controllers.DetailStore) //StoreController.go
			rg.GET("stores/:id/transaction-histories", controllers.StoreTransactionsHistories) //StoreController.go
			rg.GET("stores/:id/shift-histories", controllers.StoreShiftsHistories) //StoreController.go
			rg.POST("stores", controllers.CreateStore) //StoreController.go
			rg.PUT("stores/:id", controllers.UpdateStore) //StoreController.go

			//========================================
			// TRANSACTION
			//========================================
			rg.GET("transactions/all", controllers.GetAllTransactions) //TransactionController.go

			//========================================
			// USER
			//========================================
			rg.GET("users", controllers.GetUsers) //UserController.go
			rg.GET("users/:id", controllers.DetailUser) //UserController.go
			rg.POST("users", controllers.CreateUser) //UserController.go
			rg.PUT("users/:id", controllers.UpdateUser) //UserController.go
			rg.DELETE("users/:id", controllers.DeleteUser) //UserController.go

			//========================================
			// MIGRATE HISTORY
			//========================================
			rg.GET("migrate-history", controllers.ListMigrateHistories) //MigrateController.go

			//========================================
			// INVENTORY / PRODUK
			//========================================
			rg.GET("products", controllers.ListAllProducts) //ProductController.go

		})
	}

	// wms service integration
	wmsService := api.Group("")
	wmsService.Use(middleware.OAuthCheck())
	{
		//========================================
		// SYNC STORE
		//========================================
		wmsService.GET("destination-stores/sync", controllers.ListStoresForSync) //StoreController.go
		wmsService.POST("products/store", controllers.ReceiveMigrateDocument) //ProductController.go
		wmsService.DELETE("products-bkl", controllers.DeleteProdukBKL) //ProductController.go
	}
}