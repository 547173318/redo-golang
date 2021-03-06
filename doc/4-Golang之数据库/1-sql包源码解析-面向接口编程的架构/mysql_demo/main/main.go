package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
)
// 定义一个全局对象db
var db *sql.DB

// 实体类
type user struct {
	id   int
	age  int
	name string
}

// 定义一个初始化数据库的函数
func initDB() (err error) {
	// DSN:Data Source Name
	dsn := "root:123456@tcp(127.0.0.1:3306)/goStudy_mysql_demo?charset=utf8mb4&parseTime=True"

	// 不会校验账号密码是否正确.由Ping来校验
	// 注意！！！这里不要使用:=，我们是给全局变量赋值，然后在main函数中使用全局变量db
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return err
	}

	// 尝试与数据库建立连接（校验dsn是否正确）
	err = db.Ping()
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := initDB() // 调用输出化数据库的函数
	if err != nil {
		fmt.Printf("init db failed,err:%v\n", err)
		return
	}
	defer db.Close()
	fmt.Printf("hello mysql\n")

	//queryRowDemo()
	//queryMultiRowDemo()
	//insertRowDemo()
	//updateRowDemo()
	//deleteRowDemo()
	prepareQueryDemo()
}

// 单行查询
func queryRowDemo(){
	sqlStr := "select id,age,name from user where id=?"
	var u user
	/* 1、非常重要：确保QueryRow之后调用Scan方法，否则持有的数据库链接不会被释放
	 * 2、这个连接指的是 func (db *DB) SetMaxOpenConns(n int)所设置的连接数
	 * 3、Scan里面会调用defer r.rows.Close()，进行释放
	 */
	err := db.QueryRow(sqlStr,1).Scan(&u.id,&u.age,&u.name)
	if err != nil {
		fmt.Printf("Scan failed,err:%v\n",err)
		return
	}
	fmt.Printf("id:%d name:%s age:%d",u.id,u.name,u.age)
}

// 查询多条数据
func queryMultiRowDemo(){
	sqlStr := "select id,age,name from user where id>?"

	// 注意QueryRow和Query的区别
	// 前者如果查找不到会err，后者查找不到不会err
	rows,err := db.Query(sqlStr,0)
	if err != nil {
		fmt.Printf("query failed,err:%v\n",err)
		return
	}
	// 非常重要：关闭rows释放持有的数据库链接
	// 虽然下面的for中Scan都调用了,但是有可能Scan发出panic，就不会执行Close导致连接没有释放
	defer rows.Close()

	// 循环读取结果集中的数据
	for rows.Next() {
		var u user
		err := rows.Scan(&u.id,&u.age,&u.name)
		if err != nil {
			fmt.Printf("Scan failed,err:%v\n",err)
			return
		}
		fmt.Printf("id:%d name:%s age:%d\n",u.id,u.name,u.age)
	}
}

// 插入数据
func insertRowDemo(){
	sqlStr := "insert into user(name,age) values (?,?)"
	ret, err := db.Exec(sqlStr,"老k教练",23)
	if err != nil {
		fmt.Printf("Exec failed,err:%v\n",err)
		return
	}
	theId,err := ret.LastInsertId()
	if err != nil {
		fmt.Printf("get lastInsertId failed,err:%v\n",err)
		return
	}
	fmt.Printf("suc id:%d\n",theId)
}

// 更新数据
func updateRowDemo() {
	sqlStr := "update user set age=? where id = ?"
	ret, err := db.Exec(sqlStr, 100, 3)
	if err != nil {
		fmt.Printf("update failed, err:%v\n", err)
		return
	}
	n, err := ret.RowsAffected() // 操作影响的行数
	if err != nil {
		fmt.Printf("get RowsAffected failed, err:%v\n", err)
		return
	}
	fmt.Printf("update success, affected rows:%d\n", n)
}

// 删除数据
func deleteRowDemo() {
	sqlStr := "delete from user where id = ?"
	ret, err := db.Exec(sqlStr, 5)
	if err != nil {
		fmt.Printf("delete failed, err:%v\n", err)
		return
	}
	n, err := ret.RowsAffected() // 操作影响的行数
	if err != nil {
		fmt.Printf("get RowsAffected failed, err:%v\n", err)
		return
	}
	fmt.Printf("delete success, affected rows:%d\n", n)
}

func prepareQueryDemo(){
	sqlStr := "select id,age,name from user where id=?"
	stmt,err := db.Prepare(sqlStr)
	if err != nil {
		fmt.Printf("prepare failed,err:%v\n",err)
		return
	}
	defer stmt.Close()
	rows,err := stmt.Query(1)
	if err != nil {
		fmt.Printf("query failed, err:%v\n", err)
		return
	}
	defer rows.Close()
	for rows.Next(){
		var u user
		err := rows.Scan(&u.id,&u.age,&u.name)
		if err != nil {
			fmt.Printf("scan failed, err:%v\n", err)
			return
		}
		fmt.Printf("id:%d name:%s age:%d\n", u.id, u.name, u.age)
	}
}

























