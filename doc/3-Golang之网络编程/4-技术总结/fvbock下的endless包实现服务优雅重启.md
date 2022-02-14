## 1、前言
* 当go语言开发的server应用已经在运行时，如果更新了代码，直接编译并运行，那么不好意思，端口已经在使用中：
```
listen tcp :8000: bind: address already in use
```
* 看到这样的错误信息，我们通常都是一通下意识的操作：
```
lsof -i:8000
kill -9 …
```
* 这样做端口被占用的问题是解决了，go程序也成功更新了。但是这里面还隐藏着两个问题：
    * kill程序时可能把正在处理的用户请求给中断了
    * 从kill到重新运行程序这段时间里没有应用在处理用户请求
* 关于如何解决这两个问题，网上有多种解决方案，今天我们谈谈endless的解决方案。

## 2、endless
#### 2-1 介绍
* endless的github地址为：https://github.com/fvbock/endless
* 她的解决方案：
  * fork一个进程运行新编译的应用，该子进程接收从父进程传来的相关文件描述符，直接复用socket，同时父进程关闭socket。**父进程留在后台处理未处理完的用户请求**，这样一来问题1解决了。
  * 且复用soket也直接解决了问题2，实现0切换时间差。复用socket可以说是endless方案的核心。

* endless可以很方便的接入已经写好的程序，对于原生api，直接替换ListenAndServe为endless的方法，如下。并在编译完新的程序后，执行kill -1 旧进程id，旧进程便会fork一个进程运行新编译的程序。
>注：此处需要保证新编译的程序的路径和程序名和旧程序的一致。
```
func handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("WORLD!"))
}

func main() {
	mux1 := mux.NewRouter()
	mux1.HandleFunc("/hello", handler).
		Methods("GET")

	err := endless.ListenAndServe("localhost:4242", mux1)
	if err != nil {
		log.Println(err)
	}
	log.Println("Server on 4242 stopped")

	os.Exit(0)
}
```
* 对于使用gin框架的程序，可以以下面的方式接入：
```
    r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.String(200, config.Config.Server.AppId)
	})
	s := endless.NewServer(":8080", r)
	err := s.ListenAndServe()
	if err != nil {
		log.Printf("server err: %v", err)
	}
```
#### 2-2 原理
* 其使用非常简单，实现代码也很少，但是很强大，下面我们看看她的实现：
* 结构体
```
type endlessServer struct {
    // 用于继承 http.Server 结构
    http.Server
    // 监听客户端请求的 Listener
    EndlessListener  net.Listener  
    // 用于记录还有多少客户端请求没有完成
    wg               sync.WaitGroup
    // 用于接收信号的管道
    sigChan          chan os.Signal
    // 用于重启时标志本进程是否是为一个新进程
    isChild          bool
    // 当前进程的状态
    state            uint8 
    ...
}
```
* 这个 endlessServer 除了继承 http.Server 所有字段以外，因为还需要监听信号以及判断是不是一个新的进程，所以添加了几个状态位的字段：
    * wg：标记还有多少客户端请求没有完成；
    * sigChan：用于接收信号的管道；
    * isChild：用于重启时标志本进程是否是为一个新进程；
    * state：当前进程的状态。
* 下面我们看看如何初始化 endlessServer ：
```
func NewServer(addr string, handler http.Handler) (srv *endlessServer) {
    runningServerReg.Lock()
    defer runningServerReg.Unlock()

    socketOrder = os.Getenv("ENDLESS_SOCKET_ORDER")
    
    // 根据环境变量判断是不是子进程
    isChild = os.Getenv("ENDLESS_CONTINUE") != "" 
    
    // 由于支持多 server，所以这里需要设置一下 server 的顺序
    if len(socketOrder) > 0 {
        for i, addr := range strings.Split(socketOrder, ",") {
            socketPtrOffsetMap[addr] = uint(i)
        }
    } else {
        socketPtrOffsetMap[addr] = uint(len(runningServersOrder))
    }

    srv = &endlessServer{
        wg:      sync.WaitGroup{},
        sigChan: make(chan os.Signal),
        isChild: isChild,
        ...
        state: STATE_INIT,
        lock:  &sync.RWMutex{},
    }

    srv.Server.Addr = addr
    srv.Server.ReadTimeout = DefaultReadTimeOut
    srv.Server.WriteTimeout = DefaultWriteTimeOut
    srv.Server.MaxHeaderBytes = DefaultMaxHeaderBytes
    srv.Server.Handler = handler

    runningServers[addr] = srv
    ...
    return
}
```
* 这里初始化都是我们在 net/http 里面看到的一些常见的参数，包括 ReadTimeout 读取超时时间、WriteTimeout 写入超时时间、Handler 请求处理器等，不熟悉的可以看一下这篇：《 一文说透 Go 语言 HTTP 标准库 https://www.luozhiyun.com/archives/561 》。

* 需要注意的是，这里是通过 ENDLESS_CONTINUE 环境变量来判断是否是个子进程，这个环境变量会在 fork 子进程的时候写入。因为 endless 是支持多 server 的，所以需要用 ENDLESS_SOCKET_ORDER变量来判断一下 server 的顺序。

* 我们看看程序启动之后做了什么 ListenAndServe
```
func (srv *endlessServer) ListenAndServe() (err error) {
    addr := srv.Addr
    if addr == "" {
        addr = ":http"
    }
    
    // 异步处理信号量
    go srv.handleSignals()
    
    // 获取端口监听
    l, err := srv.getListener(addr)
    if err != nil {
        log.Println(err)
        return
    }
    
    // 将监听转为 endlessListener
    srv.EndlessListener = newEndlessListener(l, srv)

    // 如果是子进程，那么发送 SIGTERM 信号给父进程
    if srv.isChild {
        syscall.Kill(syscall.Getppid(), syscall.SIGTERM)
    }

    srv.BeforeBegin(srv.Addr)
    // 响应Listener监听，执行对应请求逻辑
    return srv.Serve()
}
```
* 这个方法其实和 net/http 库是比较像的，首先获取端口监听，然后调用 Serve 处理请求发送过来的数据，大家可以打开文章《 一文说透 Go 语言 HTTP 标准库 https://www.luozhiyun.com/archives/561 》对比一下和 endless 的异同。

* 但是还是有几点不一样的，endless 为了做到平滑重启需要用到信号监听处理，并且在 getListener 的时候也不一样，如果是子进程需要继承到父进程的 listen fd，这样才能做到不关闭监听的端口

* endless的使用方法是先编译新程序，并执行"kill -1 旧进程id"，我们看看旧程序接收到-1信号之后作了什么：

```
/*
handleSignals listens for os Signals and calls any hooked in function that the
user had registered with the signal.
*/
func (srv *endlessServer) handleSignals() {
    var sig os.Signal
    
    // 注册信号监听
    signal.Notify(
        srv.sigChan,
        hookableSignals...,
    )
    
    // 获取pid
    pid := syscall.Getpid()
    for {
        sig = <-srv.sigChan
        // 在处理信号之前触发hook
        srv.signalHooks(PRE_SIGNAL, sig)
        switch sig {
        
        // 接收到平滑重启信号
        case syscall.SIGHUP:
            log.Println(pid, "Received SIGHUP. forking.")
            err := srv.fork()
            if err != nil {
                log.Println("Fork err:", err)
            } 
        
        // 停机信号
        case syscall.SIGINT:
            log.Println(pid, "Received SIGINT.")
            srv.shutdown()
        
        // 停机信号
        case syscall.SIGTERM:
            log.Println(pid, "Received SIGTERM.")
            srv.shutdown()
        ...
        
        // 在处理信号之后触发hook
        srv.signalHooks(POST_SIGNAL, sig)
    }
}
```
* 这一部分的代码十分简洁，当我们用kill -1 $pid 的时候这里 srv.sigChan 就会接收到相应的信号，并进入到 case syscall.SIGHUP 这块逻辑代码中。

* 需要注意的是，在上面的 ListenAndServe 方法中子进程会像父进程发送 syscall.SIGTERM 信号也会在这里被处理，执行的是 shutdown 停机逻辑
```
func (srv *endlessServer) fork() (err error) {
    runningServerReg.Lock()
    defer runningServerReg.Unlock()

    // 校验是否已经fork过
    if runningServersForked {
        return errors.New("Another process already forked. Ignoring this one.")
    } 
    runningServersForked = true

    var files = make([]*os.File, len(runningServers))
    var orderArgs = make([]string, len(runningServers))
    
    // 因为有多 server 的情况，所以获取所有 listen fd
    for _, srvPtr := range runningServers { 
        switch srvPtr.EndlessListener.(type) {
        case *endlessListener: 
            files[socketPtrOffsetMap[srvPtr.Server.Addr]] = srvPtr.EndlessListener.(*endlessListener).File()
        default: 
            files[socketPtrOffsetMap[srvPtr.Server.Addr]] = srvPtr.tlsInnerListener.File()
        }
        orderArgs[socketPtrOffsetMap[srvPtr.Server.Addr]] = srvPtr.Server.Addr
    }
    
    // 环境变量
    env := append(
        os.Environ(),
    
        // 启动endless 的时候，会根据这个参数来判断是否是子进程
        "ENDLESS_CONTINUE=1",
    )
    if len(runningServers) > 1 {
        env = append(env, fmt.Sprintf(`ENDLESS_SOCKET_ORDER=%s`, strings.Join(orderArgs, ",")))
    }

    // 程序运行路径
    path := os.Args[0]
    var args []string
    // 参数
    if len(os.Args) > 1 {
        args = os.Args[1:]
    }

    cmd := exec.Command(path, args...)
    // 标准输出
    cmd.Stdout = os.Stdout
    // 错误
    cmd.Stderr = os.Stderr
    cmd.ExtraFiles = files
    cmd.Env = env  
    err = cmd.Start()
    if err != nil {
        log.Fatalf("Restart: Failed to launch, error: %v", err)
    } 
    return
}
```
* fork 这块代码首先会根据 server 来获取不同的 listen fd 然后封装到 files 列表中，然后在调用 cmd 的时候将文件描述符传入到 ExtraFiles 参数中，这样子进程就可以**无缝托管到父进程监听的端口。**

* 需要注意的是，env 参数列表中有一个 ENDLESS_CONTINUE 参数，这个参数会在 endless 启动的时候做校验：判断是否为子进程
```
func NewServer(addr string, handler http.Handler) (srv *endlessServer) {
    runningServerReg.Lock()
    defer runningServerReg.Unlock()

    socketOrder = os.Getenv("ENDLESS_SOCKET_ORDER")
    isChild = os.Getenv("ENDLESS_CONTINUE") != ""
  ...
}
```
* 所以重新编译成程序名必须和一开始使用的程序名一致, 准确的说 fork 时使用了最开始运行命令的所有参数, 必须完全一致

#### 2-3 复用socket
* 前面提到复用socket是endless的核心，必须在Serve前准备好，否则会导致**端口已使用**的异常。复用socket的实现在上面的getListener方法中：
```
func (srv *endlessServer) getListener(laddr string) (l net.Listener, err error) {
    // 如果是子进程
    if srv.isChild {
        var ptrOffset uint = 0
        runningServerReg.RLock()
        defer runningServerReg.RUnlock()
        
        // 这里还是处理多个 server 的情况
        if len(socketPtrOffsetMap) > 0 {
            
            // 根据server 的顺序来获取 listen fd 的序号
            ptrOffset = socketPtrOffsetMap[laddr] 
        }
        // fd 0，1，2是预留给 标准输入、输出和错误的，所以从3开始
        // 多文件
        f := os.NewFile(uintptr(3+ptrOffset), "")
        // 监听文件
        l, err = net.FileListener(f)
        if err != nil {
            err = fmt.Errorf("net.FileListener error: %v", err)
            return
        }
    } else {
        // 父进程 直接返回 listener
        l, err = net.Listen("tcp", laddr)
        if err != nil {
            err = fmt.Errorf("net.Listen error: %v", err)
            return
        }
    }
    return
}
``` 
* 这里如果是父进程没什么好说的，直接创建一个端口监听并返回就好了。

* 但是对于子进程来说是有一些绕，首先说一下 os.NewFile 的参数为什么要从3开始。因为子进程在继承父进程的 fd 的时候0，1，2是预留给 标准输入、输出和错误的，所以父进程给的第一个fd在子进程里顺序排就是从3开始了，又因为 fork 的时候cmd.ExtraFiles 参数传入的是一个 files，如果有多个 server 那么会依次从3开始递增。
## 4 总结
* 如何验证优雅重启的效果呢？我们通过执行kill -1 pid命令发送syscall.SIGINT来通知程序优雅重启，具体做法如下：
    * 打开终端，go build -o graceful_restart编译并执行./graceful_restart,终端输出当前pid(假设为43682)
    * 将代码中处理请求函数返回的hello gin!修改为hello q1mi!，再次编译go build -o graceful_restart
    * 打开一个浏览器，访问127.0.0.1:8080/，此时浏览器白屏等待服务端返回响应。
    * 在终端**迅速执行kill -**1 43682命令给程序发送syscall.SIGHUP信号（restart）
    * 等第3步浏览器收到响应信息hello gin!后再次访问127.0.0.1:8080/会收到hello q1mi!的响应。
* 在不影响当前未处理完请求的同时完成了程序代码的替换，实现了优雅重启。
* 但是需要注意的是，此时程序的PID变化了，因为endless 是通过fork子进程处理新请求，待原进程处理完当前请求后再退出的方式实现优雅重启的。所以当你的项目是使用类似supervisor的软件管理进程时就不适用这种方式了（因为当父进程挂掉之后，supervisor会重新拉起，导致冲突）
* 请求一定都被父进程和子进程接到了，不会丢失请求