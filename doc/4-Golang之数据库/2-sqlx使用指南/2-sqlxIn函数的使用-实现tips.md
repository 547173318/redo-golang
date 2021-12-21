## 1、前置条件
* 表结构
```
CREATE TABLE `user` (
    `id` BIGINT(20) NOT NULL AUTO_INCREMENT,
    `name` VARCHAR(20) DEFAULT '',
    `age` INT(11) DEFAULT '0',
    PRIMARY KEY(`id`)
)ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4;
```
* 结构体
```
//定义一个user结构体，字段通过tag与数据库中user表的列一致。
type User struct {
	Name string `db:"name"`
	Age  int    `db:"age"`
}
```
* bindvars（绑定变量）
    * 查询占位符?在内部称为bindvars（查询占位符）,它非常重要。你应该始终使用它们向数据库发送值，因为它们可以防止SQL注入攻击。
* database/sql不尝试对查询文本进行任何验证；它与编码的参数一起按原样发送到服务器。除非驱动程序实现一个特殊的接口，否则在执行之前，查询是在服务器上准备的。因此bindvars是特定于数据库的:
    * MySQL中使用?
    * PostgreSQL使用枚举的$1、$2等bindvar语法
    * SQLite中?和$1的语法都支持
    * Oracle中使用:name的语法
* bindvars的一个常见误解是，它们用来在sql语句中插入值。它们其实仅用于参数化，不允许更改SQL语句的结构。例如，使用bindvars尝试参数化列或表名将不起作用：
```
// ？不能用来插入表名（做SQL语句中表名的占位符）
db.Query("SELECT * FROM ?", "mytable")
 
// ？也不能用来插入列名（做SQL语句中列名的占位符）
db.Query("SELECT ?, ? FROM people", "name", "location")
自己拼接语句实现批量插入
```
## 2、实现过程
#### 2-1 手动实现批量插入
* 拼接字符串
```
// BatchInsertUsers 自行构造批量插入的语句,有点笨重
func BatchInsertUsers(users []*User) error {

	// 存放 (?, ?) 的slice
	valueStrings := make([]string, 0, len(users))
	// 存放values的slice
	valueArgs := make([]interface{}, 0, len(users) * 2)
	// 遍历users准备相关数据
	for _, u := range users {
		// 此处占位符要与插入值的个数对应
		valueStrings = append(valueStrings, "(?, ?)")

		// 手动的情况下name1 age1 name2 age2 name3 age3...
		// 一定要算好个数和？一一对应，否者无法修改数据库
		valueArgs = append(valueArgs, u.Name)
		valueArgs = append(valueArgs, u.Age)

	}
	// 自行拼接要执行的具体语句
	// stmt = (?, ?),(?, ?),(?, ?)
	stmt := fmt.Sprintf("INSERT INTO user (name, age) VALUES %s",
		strings.Join(valueStrings, ","))
	//fmt.Printf(stmt)
	_, err := db.Exec(stmt, valueArgs...)
	return err
}
```
* 手动的情况下name1 age1 name2 age2 name3 age3...
* 一定要算好个数和？一一对应，否者无法修改数据库

#### 2-2 sqlx.IN实现批量插入
* 不用花时间在拼接上面
```
// BatchInsertUsers2 使用sqlx.In帮我们拼接语句和参数, 注意传入的参数是[]interface{}
func BatchInsertUsers2(users []interface{}) error {
	query, args, _ := sqlx.In(
		"INSERT INTO user (name, age) VALUES (?), (?), (?)",
		users..., // 如果arg实现了 driver.Valuer, sqlx.In 会通过调用 Value()来展开它，不然会{}{}{}，只会对应3个？，sql语句出错
	)
	fmt.Println(query) // 查看生成的querystring
	fmt.Println(args)  // 查看生成的args
	_, err := db.Exec(query, args...)
	return err
}

// 展开结构体参数到最小数据元，变成手动拼接那样子
func (u User) Value() (driver.Value, error) {
	return []interface{}{u.Name, u.Age}, nil
}
```
* arg是否实现drive.Value的
    * 没有实现：
      * fmt.Println(query):`INSERT INTO user (name, age) VALUES (?), (?), (?)`
      * fmt.Println(args):`[{user1 11} {user2 12} {user3 13}]`
    * 有实现 
      * fmt.Println(query):`INSERT INTO user (name, age) VALUES (?, ?), (?, ?), (?, ?)`
      * fmt.Println(args):`[user1 11 user2 12 user3 13]` 
  * 可以看出， sqlx.In 会通过调用 Value()来展开它，否者sql语句出错，即要展开到最小数据元（struct里面的每一个域）
#### 2-3、NamedExec
* 更加方便，不用实现drive.Value
```
// BatchInsertUsers3 使用NamedExec实现批量插入,不用实现drive.Value
func BatchInsertUsers3(users []*User) error {
	_, err := db.NamedExec("INSERT INTO user (name, age) VALUES (:name, :age)", users)
	return err
}
```
## 3、sqlx.In 查询tips
#### 3-1 根据给定ID查询
```
// QueryByIDs 根据给定ID查询
func QueryByIDs(ids []int)(users []User, err error){
	// 动态填充id
	query, args, err := sqlx.In("SELECT name, age FROM user WHERE id IN (?)", ids)
	if err != nil {
		return
	}
	// sqlx.In 返回带 `?` bindvar的查询语句, 我们使用Rebind()重新绑定它
	query = db.Rebind(query)

	err = db.Select(&users, query, args...)
	return
}
```
#### 3-2 按照指定id查询并维护顺序
* 默认是按照id顺序，现在按照指定的顺序

```
// QueryAndOrderByIDs 按照指定id查询并维护顺序
func QueryAndOrderByIDs(ids []int)(users []User, errerror){
    // 动态填充id,用于拼接成字符串，填充在order by后面
    strIDs := make([]string, 0, len(ids))
    for _, id := range ids {
        strIDs = append(strIDs, fmt.Sprintf("%d", id))
    }

    query, args, err := sqlx.In("SELECT name, ageFROM user WHERE id IN (?) ORDER BY FIND_IN_SE(id, ?)", ids, strings.Join(strIDs, ","))
    if err != nil {
        return
    }
    // sqlx.In 返回带 `?` bindvar的查询语句, 我们使Rebind()重新绑定它
    query = db.Rebind(query)
    err = db.Select(&users, query, args...)
    return
}
```
* 动态填充id,用于拼接成字符串(不可以是[]int,因为要做字符串拼接)，填充在order by后面




