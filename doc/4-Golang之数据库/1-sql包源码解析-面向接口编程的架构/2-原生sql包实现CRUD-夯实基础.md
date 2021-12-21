## 1、CRUD的实现过程
* 建表
```
CREATE TABLE `user` (
    `id` BIGINT(20) NOT NULL AUTO_INCREMENT,
    `name` VARCHAR(20) DEFAULT '',
    `age` INT(11) DEFAULT '0',
    PRIMARY KEY(`id`)
)ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4;
```
* 查询单行
```
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
```
* 确保QueryRow之后调用Scan方法，否则持有的数据库链接不会被释放
	 * 这个连接指的是` func (db *DB) SetMaxOpenConns(n int)`所设置的连接数
	 * Scan里面会调用defer r.rows.Close()，进行释放


* 查询多行
```
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
```

* 插入
```
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
```

* 更新
```
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
```

* 删除
```
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
```
## 2、mysql的预处理

#### 2-1 什么是预处理？
* 通SQL语句执行过程：
    * 客户端对SQL语句进行占位符替换得到完整的SQL语句。
    * 客户端发送完整SQL语句到MySQL服务端
    * MySQL服务端执行完整的SQL语句并将结果返回给客户端。
* 预处理执行过程：
  * 把SQL语句分成两部分，命令部分与数据部分。
  * 先把命令部分发送给MySQL服务端，MySQL服务端进行SQL预处理。
  * 然后把数据部分发送给MySQL服务端，MySQL服务端对SQL语句进行占位符替换。
  * MySQL服务端执行完整的SQL语句并将结果返回给客户端。
#### 2-2 为什么要预处理？
* 优化MySQL服务器重复执行SQL的方法，可以提升服务器性能，提前让服务器编译，一次编译多次执行，节省后续编译的成本。
* 避免SQL注入问题

#### 2-3 代码实现
```
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
```
* `func (db *DB) Prepare(query string) (*Stmt, error)
Prepare` 会先将sql语句发送给MySQL服务端，返回一个准备好的状态用于之后的查询和命令。返回值可以同时执行多个查询和命令。
> 注意stmt状态的关闭

#### 2-4 SQL注入问题
* 我们任何时候都不应该自己拼接SQL语句！
* 这里我们演示一个自行拼接SQL语句的示例，编写一个根据name字段查询user表的函数如下：
```
// sql注入示例
func sqlInjectDemo(name string) {
	sqlStr := fmt.Sprintf("select id, name, age from user where name='%s'", name)
	fmt.Printf("SQL:%s\n", sqlStr)
	var u user
	err := db.QueryRow(sqlStr).Scan(&u.id, &u.name, &u.age)
	if err != nil {
		fmt.Printf("exec failed, err:%v\n", err)
		return
	}
	fmt.Printf("user:%#v\n", u)
}
```
* 此时以下输入字符串都可以引发SQL注入问题：
```
sqlInjectDemo("xxx' or 1=1#")
sqlInjectDemo("xxx' union select * from user #")
sqlInjectDemo("xxx' and (select count(*) from user) <10 #")
```