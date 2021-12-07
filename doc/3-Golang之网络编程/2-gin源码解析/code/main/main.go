package main
import (
	"github.com/gin-gonic/gin"
	"net/http"
)
func main(){
	r := gin.Default()
	r.GET("hello", func(context *gin.Context) {
		context.JSON(http.StatusOK,"hello")
	})
	r.Run(":9090")
}
