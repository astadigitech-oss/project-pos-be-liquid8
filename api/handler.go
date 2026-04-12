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
	// Authentication handler
	api.POST("/login", controllers.Login)
	api.GET("/checkLogin", controllers.CheckToken)

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

			// Transaction
			rg.GET("transactions", controllers.GetTransactionHistories) //TransactionController.go
			rg.GET("transactions/:id", controllers.DetailTransaction) //TransactionController.go
			rg.POST("transactions/checkout", middleware.ShiftCheck(), controllers.CheckoutTransaction) //TransactionController.go
			rg.DELETE("transactions/:id", controllers.CancelTransaction) //TransactionController.go
		})
	
	/*======================= ALL ROLE =======================*/
		//========================================
		// PPN
		//========================================
		protected.GET("ppns", controllers.GetPPN) //PPNController.go
		protected.GET("ppns/:id", controllers.DetailPPN) //PPNController.go
		protected.POST("ppns", controllers.StorePPN) //PPNController.go
		protected.PUT("ppns/:id", controllers.UpdatePPN) //PPNController.go
		protected.DELETE("ppns/:id", controllers.DeletePPN) //PPNController.go
		//========================================
		// MEMBER
		//========================================
		protected.GET("members", controllers.ListAllMembers) //MemberController.go
		protected.GET("members/:id", controllers.DetailMember) //MemberController.go
		protected.POST("members", controllers.CreateMember) //MemberController.go
		protected.PUT("members/:id", controllers.UpdateMember) //MemberController.go
		protected.DELETE("members/:id", controllers.DeleteMember) //MemberController.go

	/*======================= ADMIN ONLY =======================*/
		roleGroup(protected, []string{"superadmin","admin"}, func(rg *gin.RouterGroup) {
			// DASHBOARD
			rg.GET("dashboard", controllers.GetDashboardData) //DashboardController.go
			// PRODUCT
			rg.GET("products", controllers.ListAllProducts) //ProductController.go
			
			//========================================
			// TRANSACTION
			//========================================
			// rg.GET("transactions/all", controllers.AllTransactions) //TransactionController.go
		})
	}
}