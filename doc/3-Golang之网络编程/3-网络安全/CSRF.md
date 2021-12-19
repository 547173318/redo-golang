## 1、案例
* 作为普通用户Alice，首先登录了我们的zoobar网站，在这个网站上看看朋友的profile，转两个bar给好朋友Cindy，另一个好朋友Diana又转给她3个。然后她发现了有个用户Bob的profile中写了一个非常引人注意的URL，是一个萌猫网站。于是Alice很开心地点进去了，看了一会儿猫咪之后，她关掉了浏览器，完全没有注意到自己的bar已经少了。

## 2、原理
* CSRF是Cross-Site Request Forgery的简称，就是跨站请求伪造。这次课我们就好好理解一下这个攻击是怎么回事。在上面的演示中，Alice是通过攻击者Bob的链接点击到了攻击网站，实际上，即使Alice首先关闭了页面或者浏览器，只要她没有点击退出网站，或者只要她的cookie还没有过期，那么她访问攻击者网站的时候，她的zoobar就会被偷走。
为了理清楚，我们得从头开始看看转发zoobar的过程。在普通用户看来，zoobar网站提供了转账功能，是为了促进朋友感情的，我自己可以选择给哪个好朋友转多少bar，除了我自己外谁也不能转。但是，从服务器的角度来看，这个transfer的过程是怎么样的？

* 结合网站的代码，我们可以发现，zoobar网站的transfer页面上，有一个表单，表单的action目标是transfer.php，然后表单中有几个项，分别是转给谁，转多少。那网站是怎么判断是谁要转bar给其他人的呢？Alice登录之后，网站就给她设了一个cookie，然后Alice每次请求一个新网页的时候，浏览器都会自动把cookie带上，帮助网站识别Alice。网站在接收到请求和Cookie之后，就进行一下数据库操作，把Alice对应的bar的个数减掉两个，把Cindy对应的个数加上两个。

* 这里我们要再强调一遍，因为cookie非常重要，所以浏览器对Cookie的必要保护是有的。它会自动判断当前网页请求的URL，会根据cookie所属的URL判断发送哪个cookie过去。譬如Alice如果在zoobar网站，然后访问到恶意网址，绝对不会出现说，恶意网址可以拿到Alice的zoobar网站cookie的情况。
  
* 也就是说，不是攻击者Bob先想办法从浏览器中骗到了Alice的Cookie，在这个实验中，攻击者没办法做到这一点。但是，攻击者仍然可以利用到Alice的cookie来完成偷窃。

* CSRF攻击是通过利用浏览器来完成的。为了维持HTTP协议和网站的正常运行，浏览器不得不每次访问网站的时候都主动的携带上自己保存的相应网站的cookie。所以，攻击者所要做的，就是主动发出去一个对目标网站的请求就可以了。这就是该攻击名字的由来——跨站请求“伪造”。本来应该是用户自己主动的行为发出请求，却被攻击者伪造了。

* 只要用户当前浏览器中还保留着目标网站如Zoobar（银行）的cookie，然后又浏览了恶意攻击网站，而攻击网站又主动发出了对目标网站的请求，浏览器就会主动地携带上目标网站的Cookie。目标网站很难区分这个请求到底是目标网站发出来的，还是攻击者发出来的。所以，有一条安全规则是：当完成网站使用后，请点击登出网站，而不是直接点右上角的×，因为登出会销毁Cookie。

* 理解了原理之后，我们来看看这个攻击的代码实现。CSRF攻击实现的主要事项是伪造这个请求。有很多种方法可以实现。首先，攻击者要伪造这个请求，必须知道zoobar网站发给服务器的请求长什么样。这个怎么知道呢？答案很简单，只要注册进入网站，这个请求是前端发送过去的，右键查看源代码，在页面上一清二楚。
```
<form method=POST name=transferform
  action="/transfer.php">
 
<p>Send <input name=zoobars type=text value="" size=5> zoobars</p>
<p>to <input name=recipient type=text value=""></p>
<input type=submit name=submission value="Send">
</form>
```


* 所以，攻击者只要将以上transfer的页面上的表单代码拷贝一下，放到自己的网站上就可以了。
> **相对而言，如果使用GET方法提供表单确实要更简单一点，只需要构造一个URL，诱导受害者去点击就行了。不过即使使用POST，攻击者也同样可以伪造**

* 此时的攻击者网页大概长的和转账页面一样；用户当然不会傻到要在攻击者网站填上要发给attacker自己的钱物。但是没关系，攻击者可以自己写页面，把值默认填好；攻击者也不会傻到要去点击send按钮。但是攻击者绕过这一点的方法多得是。攻击者可以把按钮上的文字描述改一下，改成“点击赢大奖”，“点击赢华为Mate20”等，各位会不会有兴趣去点一下？

* 估计讲到这里，大家心里正在默默地下决心，以后打开不常用网站的网页，不管什么按钮我都不点。这个是很好的，但是足够有用吗？很可惜的是，只有这一点，还不够防御CSRF。因为之前我们在介绍JS的时候说过，JS可以控制页面是所有的内容，包括表单。也即，攻击者在自己的攻击页面上写上表单之后，完全可以自行提交。也即，只要打开页面，这个表单就提交了，浏览器还很贴心地附上了用户的cookie。

## 3、原理图示
![](img/CSRF.jpg)
* 理解：黑客发来的html和官方发来的html没有有什么区别？没有，用户都可以submit，只要是从用户浏览器发出去的请求，服务器就可以处理（cookie是根据浏览器决定的）

## 4、解决方法
#### 4-1、refer cookie
* 如果不是让浏览器来自动检查，程序员写网站的时候顺带检查一下行不行？从代码角度而言，绝对是没问题的。因为正如在之前看过的，用户发出的HTTP请求中有Referer这一项，就是表示当前的请求是从哪个网站发出去的。这样，只需要使用代码来检查一下就行。譬如这样：
```
<script>
      if((document.referrer.indexOf('localhost')<0) && (document.referrer.indexOf('zoobar.com')<0)){
              alert(document.referrer);
              document.location = "http://www.baidu.com";
              top.location = document.location;
                
}
</script>
```
* 我们给大家演示一下就明白了，这种方法只是看起来很美好。

* 为啥嘞？因为用户请求从浏览器发出去之前，浏览器可以对它做各种修改。其中有一种，作为对自己隐私非常介意的用户，可能会主动设置自己的浏览器在发出请求的时候不带Referrer这个选项。现在的浏览器可以很容易控制不发送referfer。在浏览器URL中输入about:config；然后在search中输入referer；可以找到
  * 0 – Disable referrer.
  * 1 – Send the Referer header when clicking on a link, and set document.referrer for the following page.
  * 2 – Send the Referer header when clicking on a link or loading an image 

#### 4-2、验证码
* 这种方法是强制用户在转账之前必须进行交互，也即需要一个验证码，这对于银行转账而言还是非常重要的。但是考虑到用户体验友好，不能给所有的操作都加上验证码。因此验证码只能作为一种辅助手段，不能作为主要解决方案。【即使有验证码，当骗子的目光集中到验证码的时候，连验证码都能骗走。】

#### 4-3、Anti CSRF Token

* 攻击者能够攻击成功，一方面是浏览器热心发送了cookie，一方面是攻击者很容易伪造请求。那有没有可能让攻击者不能伪造请求？给攻击者的请求伪造设置门槛？这个防御思路，把改进的方向调整到了网站本身。CSRF攻击能够成功，是因为请求太容易伪造了；如果请求中包括一些攻击者不能容易获取的信息，那么攻击自然不能成功。当然，网页本身还是所有人可见的，那么就需要让其中的信息难以简单复制。换言之，我们需要在表单中添加一些信息，这个信息最好是用户独特而且快速变化的，这样攻击者及时能看到一个人的信息，他也不能伪造其他人的；及时他获得了其他人的信息，但是这个信息快速变化，短时间不用便会过期。另外，添加这样的信息最好不要影响正常用户的使用，维持用户友好。

* 幸好，这样的事情其实很容易做到。现在业界对CSRF的防御，就是这个思路，使用一个Token（Anti CSRF Token）。

* 用户登录网站，服务端生成一个Token，放在用户的Session
在页面表单附带上Token参数，为了不影响用户，可以设置type=hidden
用户提交请求时，表单中的这一参数会自动提交， 服务端验证表单中的Token是否与用户Session中的Token一致，一致为合法请求，不是则非法请求
每次提交，token值可以更新
因为这个Token值是每个用户不同，并且开发人员可以设置粒度，用户每次登录不同，还是每次提交不同，这样彻底使得攻击者无法伪造请求。

最终的页面可能长这个样子：
```
<form method=POST name=transferform   action="/transfer.php">
 
<input name=token type=hidden value="09d6dde682f36904cd58e43cd0e03d59">
<p>Send <input name=zoobars type=text value="" size=5> zoobars</p>
<p>to <input name=recipient type=text value=""></p>
<input type=submit name=submission value="Send">
</form>
```

* 在合法的网站上提交请求的时候，带的参数如上；因为攻击者无法获得每个用户每次的token值，所以攻击失败。
* **这就使得，黑客发给用户的html于服务器的html不同了，浏览器可以标识，到底是不是从官方给的html表单来发送请求的，就算黑客抄袭代码，token值也与用户不一样了**