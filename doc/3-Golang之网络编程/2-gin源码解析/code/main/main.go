package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
)

func fun1(c *gin.Context){
	fmt.Println("fun1 start")
	fmt.Println("fun1 end")
}
func fun2(c *gin.Context){
	fmt.Println("fun2 start")
	c.Set("key","val")
	fmt.Println("fun2 end")
}
func fun3(c *gin.Context){
	fmt.Println("fun3 start")
	val,ok := c.Get("key")
	if ok {
		fmt.Println(val.(string))
	}
	fmt.Println("fun3 end")
}
func fun4(c *gin.Context){
	fmt.Println("fun4 start")
	fmt.Println("fun4 end")
}

func main(){
	r := gin.Default()

	//r.GET("/hello", func(context *gin.Context) {
	//	context.JSON(http.StatusOK,"hello")
	//})

	group1 := r.Group("/group1",fun1)
	group1.Use(fun2)
	{
		group1.GET("/get",fun3,fun4)
	}

	r.Run(":9090")
}
