## 1、GMP源码解析
#### 1-1 G
* G是Goroutine的缩写，相当于操作系统中的进程控制块(不过这是在用户层面的PCB)，在这里就是Goroutine的控制结构，是对Goroutine的抽象。其中包括**执行的函数指令及参数**；G保存的任务对象；线程上下文切换，现场保护和现场恢复需要的寄存器(SP、IP)等信息。

* Go不同版本Goroutine默认栈大小不同。
```
// Go1.11版本默认stack大小为2KB
_StackMin = 2048
```
* 下面这个函数，看完整篇文章，就理解了（可以跳过）
```
// 创建一个g对象,然后放到g队列
// 等待被执行
func newproc1(fn *funcval, argp *uint8, narg int32, callergp *g, callerpc uintptr) {
    _g_ := getg()

    _g_.m.locks++
    siz := narg
    siz = (siz + 7) &^ 7

    _p_ := _g_.m.p.ptr()
    newg := gfget(_p_)    
    if newg == nil {        
       // 初始化g stack大小
        newg = malg(_StackMin)
        casgstatus(newg, _Gidle, _Gdead) // 
        allgadd(newg)
    }    
    // 以下省略}
```
* G: 代表一个goroutine对象，每次go调用的时候，都会创建一个G对象，它包括栈、指令指针以及对于调用goroutines很重要的其它信息，比如阻塞它的任何channel，其主要数据结构：
```
type g struct {
  stack       stack   // 描述了真实的栈内存，包括上下界

  m              *m     // 当前的m
  sched          gobuf   // goroutine切换时，用于保存g的上下文      
  param          unsafe.Pointer // 用于传递参数，睡眠时其他goroutine可以设置param，唤醒时该goroutine可以获取
  atomicstatus   uint32
  stackLock      uint32 
  goid           int64  // goroutine的ID
  waitsince      int64 // g被阻塞的大体时间
  lockedm        *m     // G被锁定只在这个m上运行
}
```
* 其中最主要的当然是sched了，保存了goroutine的上下文。goroutine切换的时候不同于线程有OS来负责这部分数据，而是由一个**gobuf对象来保存**，这样能够更加轻量级(**套娃**)，再来看看gobuf的结构：
```
// 其实就是保存了当前的栈指针，计数器，当然还有g自身
// 这里记录自身g的指针是为了能快速的访问到goroutine中的信息。
type gobuf struct {
    sp   uintptr
    pc   uintptr
    g    guintptr
    ctxt unsafe.Pointer
    ret  sys.Uintreg
    lr   uintptr
    bp   uintptr // for GOEXPERIMENT=framepointer
}
```

* 在运行时系统接到一个`newproc`（编译器将go语句变成`newproc`）调用
  * 会先检查go函数及其参数的合法性，然后试图从本地P的自由G列表和调度器的自由G列表获取可用的G
  * 如果没有获取到，就新建一个G。新建的G会**第一时间被加入到全局G列表**,随后，系统会对这个G进行一次初始化，包括关联go函数以及设置该G的状态和ID等步骤
  * 在初始化完成后，这个G会立即被存储在本地P的`runnext`字段中，如果 `runnext`字段中已存有一个G，那么这个已有G就会被踢到该P的可运行G队列末尾。
    * 如果该队列已满，那么这个G就只能追加到**调度器**的可运行G队列中，具体后面会对调度器会进行介绍
* G的创建
```
func malg(stacksize int32) *g {
	newg := new(g)
	if stacksize >= 0 {
		stacksize = round2(_StackSystem + stacksize)
		systemstack(func() {
			newg.stack = stackalloc(uint32(stacksize))
		})
		newg.stackguard0 = newg.stack.lo + _StackGuard
		newg.stackguard1 = ^uintptr(0)
		// Clear the bottom word of the stack. We record g
		// there on gsignal stack during VDSO on ARM and ARM64.
		*(*uintptr)(unsafe.Pointer(newg.stack.lo)) = 0
	}
	return newg
}

```
* G的状态
  * 【1】空闲中(_Gidle): 表示G刚刚新建, 仍未初始化
  * 【2】待运行(_Grunnable): 表示G在运行队列中, 等待M取出并运行
  * 【3】运行中(_Grunning): 表示M正在运行这个G, 这时候M会拥有一个P
  * 【4】系统调用中(_Gsyscall): 表示M正在运行这个G发起的系统调用, 这时候M并不拥有P
  * 【5】等待中(_Gwaiting): 表示G在等待某些条件完成, 这时候G不在运行也不在运行队列中(可能在channel的等待队列中)
  * 【6】已中止(_Gdead): 表示G未被使用, 可能已执行完毕(并在freelist中等待下次复用),或者刚刚初始化完成，但是未被使用
  * 【7】栈复制中(_Gcopystack): 表示G正在获取一个新的栈空间并把原来的内容复制过去(用于防止GC扫描)
M的状态
* 值得一提的是进入死亡状态的G是可以被重新初始化并使用的，他们会被放入本地P或者调度器的自由G列表
* 与G有关的结构
    * 【1】全局G列表：集中存放当前运行时系统中所有G的指针
    * 【2】调度器可运行G队列：后续会详细讲解
    * 【3】调度器自由G列表：后续会详细讲解
```
var allgs    []*g //全局G列表
```
#### 1-2 M

* M是一个线程或称为Machine，所有M是有线程栈(**OS级别的线程栈**)的。如果不对该线程栈提供内存的话，系统会给该线程栈提供内存(不同操作系统提供的线程栈大小不同)。
* M：代表一个线程，**每次创建一个M的时候，都会有一个底层线程创建**；所有的G任务，最终还是在M上执行
> 个人理解：M只是go语言层面对于一个内核级线程的描述，在除了M实现过程以外的地方，都可以把M理解成**内核级线程**
* G的栈是比OS更高级别的线程栈，GM绑定的时候，则M.stack→G.stack，M的PC寄存器指向G提供的函数，然后去执行。
* 视角一：g0
```
type m struct {    
    /*
        1.  所有调用栈的Goroutine,这是一个比较特殊的Goroutine。
        2.  普通的Goroutine栈是在Heap分配的可增长的stack,而g0的stack是M对应的线程栈。
        3.  所有调度相关代码,会先切换到该Goroutine的栈再执行。
    */
    g0       *g

    curg     *g         // M当前绑定的结构体G

    // SP、PC寄存器用于现场保护和现场恢复，内核级别
    vdsoSP uintptr
    vdsoPC uintptr

    // 省略…}
```
* g0有点像“系统调用”，开始切换G，套娃就完事了

* 视角2：整体（后面**goroutine执行过程**会有详解）
```
type m struct {

   g0      *g               // OS在启动之初为M分配的一个特殊的G
   mstartfn      func()     // m的启动函数
   curg          *g         // 当前运行的g的指针
   p             puintptr   // 当前m相关联的p
   nextp         puintptr   // 临时存放p
   spinning      bool       // 自旋状态表示当前M是否在寻找可运行G 
   park          note       // 休眠锁
   schedlink     muintptr   // 链表
   ...
}
```
* 结构体M中有两个G是需要关注一下的:
  * 一个是curg，代表结构体M当前绑定的结构体G
  * 另一个是g0，是带有调度栈的goroutine
    * 这是一个比较特殊的goroutine。普通的goroutine的栈是在堆上分配的可增长的栈(跟高级别)，而g0的栈是M对应的线程的栈。所有调度相关的代码，会先切换到该goroutine的栈中再执行。
    * 也就是说goroutine的栈也是用的g实现，而不是使用的OS的
* M的创建
```
//创建一个新的m。它将以对fn或调度程序的调用开始。
// newm会新建一个m的实例, m的实例包含一个g0, 然后调用newosproc动一个系统线程
func newm(fn func(), _p_ *p) {
   
   mp := allocm(_p_, fn) // 获得g0
   
   //设置当前M的下一个P为_p_
   mp.nextp.set(_p_)
   ...
   newm1(mp) // 创建M
}
```
```
func allocm(_p_ *p, fn func()) *m {
    ...
    mp := new(m)
    mp.mstartfn = fn
    mcommoninit(mp)
    //创建g0
    // In case of cgo or Solaris or illumos or Darwin, pthread_create will make us a stack.
    // Windows and Plan 9 will layout sched stack on OS stack.
    if iscgo || GOOS == "solaris" || GOOS == "illumos" || GOOS == "windows" || GOOS == "plan9" || GOOS == "darwin" {
        //分配一个新的g
        mp.g0 = malg(-1)
    } else {
        mp.g0 = malg(8192 * sys.StackGuardMultiplier)
    }
    mp.g0.m = mp
    if _p_ == _g_.m.p.ptr() {
        releasep()
    }
    releasem(_g_.m)
    return mp
}
```
```
// 调用newosproc动一个系统线程
func newm1(mp *m) {
    if iscgo {
        //cgo的一些处理
        ...
    }
    //创建系统线程
    execLock.rlock() // Prevent process clone.
    newosproc(mp)
    execLock.runlock()
}
```
```
// newosproc会调用syscall clone创建一个新的线程
func newosproc(mp *m) {
    //g0的栈内存地址
    stk := unsafe.Pointer(mp.g0.stack.hi)
    ...
    sigprocmask(_SIG_SETMASK, &sigset_all, &oset)
    
    //系统调用
    ret := clone(cloneFlags, stk, unsafe.Pointer(mp), unsafe.Pointer(mp.g0), unsafe.Pointer(funcPC(mstart)))
    
    sigprocmask(_SIG_SETMASK, &oset, nil)
    ...
}
```
* 与M有关的结构
    * 【1】全局M列表：M在创建之初，会被加入到全局的M列表中，运行时系统会为这个M专门创建一个新的内核线程并与之相关联
    * 【2】调度器空闲M列表：M在系统停止M的时候，会被放入调度器的空闲M列表,调度器拥有的空闲M列表后续会讲
```
var  allm       *m  //全局M列表
```
* 什么时候创建新的M？
    * 当M因为系统调用而阻塞的时候，系统会把M和与之关联的P分离开来，这时，如果这个P的可运行队列中还有未被运行的G，那么系统会找到一个空闲的M或者创建一个新的M，并与该P关联以满足这些G的运行需要(hand off)，根据spinning决定是否创建
* M并没有像G和P一样的状态标记, 但可以认为一个M有以下的状态:
    * 【1】自旋中(spinning): M正在从运行队列获取G, 这时候M会拥有一个P，**（理解成马上有一M要运行了）**
    * 【2】执行go代码中: M正在执行go代码, 这时候M会拥有一个P
    * 【3】执行原生代码中: M正在执行原生代码或者阻塞的syscall, 这时M并不拥有P（M不需要cpu提供服务）
    * 【4】休眠中: M发现无待运行的G时会进入休眠, 并添加到空闲M链表中, 这时M并不拥有P
* 自旋中(spinning)这个状态非常重要, 是否需要唤醒或者创建新的M取决于当前自旋中的M的数量
> 因为我们要有足量的M使得cpu不会空闲，而又不会有太多的M导致切换开销过高，所以spinning的M的多少说明有多少M马上要开始

#### 1-3 P

* P(Processor)是一个抽象的概念，并不是真正的物理CPU。所以当P有任务时需要创建或者唤醒一个系统线程来执行它队列里的任务。所以P/M需要进行绑定，构成一个执行单元。
* Go运行时的调度器使用`GOMAXPROCS`参数来确定需要使用多少个OS线程来同时执行Go代码。默认值是机器上的CPU核心数。例如在一个8核心的机器上，调度器会把Go代码同时调度到8个OS线程上（`GOMAXPROCS`是m:n调度中的n）。
> m:n不是用户级线程和内核线程的对应关系吗?怎么有何P有关系了？因为P和M是一一对应的，尽管可能有空闲、阻塞的M，但是每时每刻在运行的M一定等于P的数量

* P决定了同时可以并发任务的数量，可通过`GOMAXPROCS`限制同时执行用户级任务的操作系统线程。可以通过runtime.GOMAXPROCS进行指定。在Go1.5之后`GOMAXPROCS`被默认设置可用的核数，而之前则默认为1。
* `GOMAXPROCS()`源码实现
```
// 自定义设置GOMAXPROCS数量
func GOMAXPROCS(n int) int {    
    /*
        1.  GOMAXPROCS设置可执行的CPU的最大数量,同时返回之前的设置。
        2.  如果n < 1,则不更改当前的值。
    */
    ret := int(gomaxprocs)

    stopTheWorld("GOMAXPROCS")    
    // startTheWorld启动时,使用newprocs。
    newprocs = int32(n)
    startTheWorld()    
    return ret
}

// 默认P被绑定到所有CPU核上
// P == cpu.cores
```
* 获取处理器数量源码实现
```
func getproccount() int32 {    
    const maxCPUs = 64 * 1024
    var buf [maxCPUs / 8]byte


    // 获取CPU Core
    r := sched_getaffinity(0, unsafe.Sizeof(buf), &buf[0])

    n := int32(0)    
    for _, v := range buf[:r] {        
       for v != 0 {
            n += int32(v & 1)
            v >>= 1
        }
    }    
    if n == 0 {
       n = 1
    }    
    return n
}
// 一个进程默认被绑定在所有CPU核上,返回所有CPU core。
// 获取进程的CPU亲和性掩码系统调用
// rax 204                          ; 系统调用码
// system_call sys_sched_getaffinity; 系统调用名称
// rid  pid                         ; 进程号
// rsi unsigned int len             
// rdx unsigned long *user_mask_ptr
sys_linux_amd64.s:
TEXT runtime·sched_getaffinity(SB),NOSPLIT,$0
    MOVQ    pid+0(FP), DI
    MOVQ    len+8(FP), SI
    MOVQ    buf+16(FP), DX
    MOVL    $SYS_sched_getaffinity, AX
    SYSCALL
    MOVL    AX, ret+24(FP)
    RET
```
* P：代表一个处理器，每一个运行的M都必须绑定一个P，就像线程必须在某一个CPU核上执行一样，由P来调度G在M上的运行，P的个数就是GOMAXPROCS（最大256），启动时固定的，一般不修改
* M的个数和P的个数不一定一样多（会有休眠的M或者不需要太多的M）（最大10000）；每一个P保存着本地G任务队列，也有一个全局G任务队列。P的数据结构：
```
type p struct {
    lock mutex

    id          int32
    status      uint32 // 状态，可以为pidle/prunning/...
    link        puintptr
    schedtick   uint32     // 每调度一次加1
    syscalltick uint32     // 每一次系统调用加1
    sysmontick  sysmontick 
    m           muintptr   // 回链到关联的m
    mcache      *mcache
    racectx     uintptr

    goidcache    uint64 // goroutine的ID的缓存
    goidcacheend uint64

    // 可运行的goroutine的队列
    runqhead uint32
    runqtail uint32
    runq     [256]guintptr

    runnext guintptr // 下一个运行的g

    sudogcache []*sudog
    sudogbuf   [128]*sudog

    palloc persistentAlloc // per-P to avoid mutex

    pad [sys.CacheLineSize]byte

    gFree struct { gList n int32 } // 自由G链表
}

```
* P的状态：
  * 【1】空闲中(_Pidle): 当M发现无待运行的G时会进入休眠, 这时M拥有的P会变为空闲并加到空闲P链表中
  * 【2】运行中(_Prunning): 当M拥有了一个P后, 这个P的状态就会变为运行中, M运行G会使用这个P中的资源
  * 【3】系统调用中(_Psyscall): 当go调用原生代码, 原生代码又反过来调用go代码时, 使用的P会变为此状态
  * 【4】GC停止中(_Pgcstop): 当gc停止了整个世界(STW)时, P会变为此状态，或者`init`之后又
  * 【5】已中止(_Pdead): 当P的数量在运行时改变, 且数量减少时多余的P会变为此状态
* P自带的一些数据结构：
    * 【1】可运行G队列（`runqhead`）：一个G被启用后，会先被追加到某个P的可运行G队列中，运行时系统会把P中的可运行G全部取出，并放入调度器的可运行G队列中，**被转移的G会在以后经由调度再次放入某个P的可运行G队列**
    > P优先从内部获取执行的g，这样能够提高效率
    * 【2】自由G链表(`gFree`)：这个列表包含了一些已经完成的G，当一个go语句欲启动一个G的时候，系统会试图从相应P的自由G队列获取一个G来封装这个fn，仅当获取不到这样一个G的时候才会创建一个新的G，具体可以看后面的`go func()`执行过程

* 关于P的一些数据结构：
    * 【1】全局P列表：包含了当前运行时系统创建的所有P调度器
    * 【2】空闲P列表：当一个P不在于任何M相关联，运行时系统就会把它放入该列表，而当运行时系统需要一个空闲P关联某个M时，会从该列表取出一个；P进入空闲列表的前提是他的可运行G列表必须为空

* P的创建(`procresize`)
    * 在调度器初始化的时候，会调整P的数量，这个时候所有的P都是调用`procresize`新建的，除了分配给当前主线程外，其余的P都被放入全局P列表中。
    * `procresize`默认只有调度器初始化函数`schedinit`和`startTheWorld`会调用。后文会讲到调度器初始化，而`startTheWorld`会激活全部由本地任务的P对象。

```
var  allp      *p  //全局M列表
```

```
// p结构体里面初始化init
func (pp *p) init(id int32) {
    pp.id = id
    pp.status = _Pgcstop // 创建之初的状态
    pp.sudogcache = pp.sudogbuf[:0]
    ...
    // 为P分配cache对象
    if pp.mcache == nil {
        if id == 0 {
            if mcache0 == nil {
                throw("missing mcache?")
            }
            // Use the bootstrap mcache0. Only one P will get
            // mcache0: the one with ID 0.
            pp.mcache = mcache0
        } else {
            //创建cache
            pp.mcache = allocmcache()
        }
    }
    ...
}
```
* 可以看到P创建之初的状态是`Pgstop`，在接下来的初始化之后(`procresize`)，系统会将其状态设置为`Pidle`
* procresize主要根据设定的p数量对全局的P列表进行*重整*,没有本地任务的会被放入空闲链表
* `procresize`函数由`schedinit()`调用
```
func procresize(nprocs int32) *p {
    old := gomaxprocs //gomaxprocs=len(allp)
    ...
    //新增P
    for i := old; i < nprocs; i++ {
        pp := allp[i]
        if pp == nil {
            pp = new(p)
        }
        //初始化P分配cache
        pp.init(i)
        //保存到allp
        atomicstorep(unsafe.Pointer(&allp[i]), unsafe.Pointer(pp))
    }
    ...
    // getg是编译器函数，它会返回当前指针指向的g
    _g_ := getg() 

    // 重新调整g在哪一个p上面
    if _g_.m.p != 0 && _g_.m.p.ptr().id < nprocs {
    	// 继续使用当前P
    	_g_.m.p.ptr().status = _Prunning
    	_g_.m.p.ptr().mcache.prepareForSweep()
    } else {
        // 释放当前P，获取allp[0]。     
        if _g_.m.p != 0 {
            ...
            _g_.m.p.ptr().m = 0
        }
        _g_.m.p = 0
        p := allp[0]
        p.m = 0
        p.status = _Pidle
        acquirep(p)
        ...
    }


    // 从未使用的P中释放资源
    for i := nprocs; i < old; i++ {
        p := allp[i]
        p.destroy()
        //无法释放P本身，因为它可以被syscall中的M引用
    }
    
    // 将没有本地任务的P放到空闲链表
    var runnablePs *p
    for i := nprocs - 1; i >= 0; i-- {
        p := allp[i]
        // 确保不是当前正在使用的P
        if _g_.m.p.ptr() == p {
            continue
        }
        p.status = _Pidle // 设置空闲状态
        if runqempty(p) {
            // 放入调度器空闲P链表
            // 没有本地任务的P
            pidleput(p)
        } else {
            //还有本地任务，构建链表
            p.m.set(mget())
            p.link.set(runnablePs)
            runnablePs = p
        }
    }
    ...
    //返回由本地任务的P
    return runnablePs

}
```
```
func (pp *p) destroy() {
    //将本地任务转移到全局队列
    for pp.runqhead != pp.runqtail { // 可以仍然还有运行的G
        // 从本地队列尾部弹出
        pp.runqtail--
        gp := pp.runq[pp.runqtail%uint32(len(pp.runq))].ptr()
        // 推到全局队列头部
        globrunqputhead(gp)
    }
    ...
    // 释放当前P绑定的cache
    freemcache(pp.mcache)
    pp.mcache = nil
    // 将当前P的G复用链转移到全局
    gfpurge(pp)
    ...
    pp.status = _Pdead
}
```
* 如果P因为`allp`的缩小而被认为是多余的，那么这些P会被`destory`，其状态会被置为`Pdead`
* 在`destory`中会调用`gfpurge`将P中**自由G链表**的G全部转移到调度器的自由G列表中，在这之前他的**可运行G队列**中的G也会转移到调度器的可运行G队列

* 为什么P的默认数量是CPU的总核心数？
    * 为了尽可能提高性能，保证n核机器上同时又n个线程**并行**运行，提高CPU利用率。

## 2、关键概念 
#### 2-1 P 队列
* P有两种队列：本地队列和全局队列
* 本地队列： 当前P的队列，本地队列是Lock-Free，没有数据竞争问题，无需加锁处理，可以提升处理速度
* 全局队列：全局队列为了保证**多个P之间任务的平衡**。所有M共享P全局队列，为保证数据竞争问题，需要加锁处理。相比本地队列处理速度要低于全局队列。

#### 2-2 上线文切换
* 简单理解为当时的环境即可，环境可以包括当时程序状态以及变量状态。例如线程切换的时候在内核会发生上下文切换，这里的上下文就包括了当时寄存器的值，把寄存器的值保存起来，等下次该线程又得到cpu时间的时候再恢复寄存器的值，这样线程才能正确运行。

* 对于代码中某个值说，上下文是指这个值所在的局部(全局)作用域对象。相对于进程而言，上下文就是进程执行时的环境，具体来说就是各个变量和数据，包括所有的寄存器变量、进程打开的文件、内存(堆栈)信息等。

#### 2-3 线程清理（用户级别的goroutine清理）
* Goroutine被调度执行必须保证P/M进行绑定，所以线程清理只需要让当前的M将P释放就可以实现线程的清理。什么时候P会释放，保证其它G可以被执行。P被释放主要有两种情况。
    * 主动释放：最典型的例子是，当执行G任务时有系统调用，当发生系统调用时M会处于Block状态。调度器会设置一个超时时间，当超时时会将P释放。

    * 被动释放：如果发生系统调用，有一个专门监控程序，进行扫描当前处于阻塞的P/M组合。当超过系统程序设置的超时时间，会自动将P资源抢走。去执行队列的其它G任务。
    > 上面两种都是hand off机制：M把P让出去

#### 2-4 关于hand off
* **hand off**：当 M 执行某一个 G 时候如果发生了`syscall`或则其余阻塞操作，M 会阻塞，如果当前有一些G 执行，`runtime`会把这个线程 M 从 P 中摘除 (`detach`)，然后再创建一个新的操作系统的线程 (如果有空闲的线程可用就复用空闲线程) 来服务于这个P
* 当 M 系统调用结束时候，这个G会尝试获取一个空闲的P执行，并放入到这 P的本地队列
  * 【1】如果顺利，**G此时是用拥有与之锁定的M的**，调用对应的M运行即可
  * 【2】如果获取不到 P，调用`dropg`函数，那么这个线程M变成休眠状态，加入到空闲线程中，然后这个G会被放入全局队列中
> 后面的schedul()调度中会详细讲到
#### 2-5 空闲M链表（有待深入理解）
* 当M发现无待运行的G时会进入休眠（如上面提到的没有空闲的P，自然也就没有M的用武之地了）, 并添加到**调度器空闲M链表**中, 空闲M链表保存在全局变量sched.
* 进入休眠的M会等待一个信号量(m.park), 唤醒休眠的M会使用这个信号量
* **go需要保证有足够的M可以运行G, 是通过这样的机制实现的:**
    * 【1】入队待运行的G后, 如果当前无自旋的M但是有空闲的P说明一定缺少M了, 就唤醒或者新建一个M
    * 【2】当M离开自旋状态，并准备运行出队的G时, 如果当前无自旋的M但是有空闲的P，说明一定缺少M了, 就唤醒或者新建一个M
    * 【3】当M离开自旋状态，并准备休眠时, 会在离开自旋状态后再次检查所有运行队列, 如果有待运行的G则重新进入自旋状态
* 因为"入队待运行的G"和"M离开自旋状态"会同时进行, go会使用这样的检查顺序:
    * 入队待运行的G => 内存屏障 => 检查当前自旋的M数量 => 唤醒或者新建一个M
    * 减少当前自旋的M数量 => 内存屏障 => 检查所有运行队列是否有待运行的G => 休眠
> 这样可以保证不会出现待运行的G入队了, 也有空闲的资源P, 但无M去执行的情况.
#### 2-6 栈扩张

* 因为go中的协程是stackful coroutine, 每一个goroutine都需要有自己的栈空间,栈空间的内容在goroutine休眠时需要保留, 待休眠完成后恢复(这时整个调用树都是完整的).
* 这样就引出了一个问题, goroutine可能会同时存在很多个, 如果每一个goroutine都预先分配一个**足够**的栈空间那么go就会使用过多的内存.
* 为了避免这个问题, go在一开始只为goroutine分配一个很小的栈空间, 它的大小在当前版本是2K，当函数发现栈空间不足时, 会申请一块新的栈空间并把原来的栈内容复制过去.

* 会检查比较rsp减去一定值以后是否比g.stackguard0小(栈是高地址到低地址，**减完之后结果小说明空间不够了**，要更加大的空间), 如果小于等于则需要调到下面调用`morestack_noctxt`函数(该函数也可用于判断抢占)
* 具体说来
    * `morestack_noctxt`函数清空`rdx`寄存器并调用`morestack`函数.
    * `morestack`函数会保存G的状态到`g.sched`, **切换到g0和g0的栈空间**, 然后调用`newstack`函数，申请更大的空间

#### 2-7 TLS
* TLS的全称是Thread-local storage, 代表每个线程的中的本地数据.
* 例如标准c中的errno就是一个典型的TLS变量, 每个线程都有一个独自的errno, 写入它不会干扰到其他线程中的值.
* **go在实现协程时非常依赖TLS机制, 会用于获取系统线程中当前的G和G所属的M的实例.**
* 因为go并不使用glibc, 操作TLS会使用系统原生的接口, 以linux x64为例,go在**新建M**时会调用`arch_prctl`这个syscall设置`FS`寄存器的值为`M.tls`的地址
* 使得**运行中每个M的FS寄存器都会指向它们对应的M实例的tls**，linux内核调度线程时FS寄存器会跟着线程一起切换
* 这样go代码只需要访问FS寄存器就可以存取线程本地的数据
#### 4-8 写屏障(Write Barrier)
* 因为go支持并行GC, GC的扫描和go代码可以同时运行, 这样带来的问题是GC扫描的过程中go代码有可能改变了对象的依赖树,例如开始扫描时发现根对象A和B, B拥有C的指针, GC先扫描A, 然后B把C的指针交给A, GC再扫描B, 这时C就不会被扫描到
* 为了避免这个问题, go在GC的**标记阶段**会启用写屏障(Write Barrier)
* 启用了写屏障(`Write Barrier`)后, 当B把C的指针交给A时, GC会认为在这一轮的扫描中C的指针是存活的,即使A可能会在稍后丢掉C, 那么C就在下一轮回收
* 写屏障只针对指针启用, 而且只在GC的标记阶段启用, 平时会直接把值写入到目标地址:
> 关于写屏障的详细将在下一篇(GC篇)分析.



## 3、GMP之外的schedt源码实现
* schedt，可以看做是一个全局的调度者
```
type schedt struct {
    goidgen  uint64
    lastpoll uint64

    lock mutex

    midle        muintptr // idle状态的m
    nmidle       int32    // idle状态的m个数
    nmidlelocked int32    // lockde状态的m个数
    mcount       int32    // 创建的m的总数
    maxmcount    int32    // m允许的最大个数

    ngsys uint32 // 系统中goroutine的数目，会自动更新

    pidle      puintptr // idle的p
    npidle     uint32
    nmspinning uint32 

    // 全局的可运行的g队列
    runqhead guintptr
    runqtail guintptr
    runqsize int32

    // dead的G的全局缓存
    gflock       mutex
    gfreeStack   *g
    gfreeNoStack *g
    ngfree       int32

    // sudog的缓存中心
    sudoglock  mutex
    sudogcache *sudog
}
```

* 调度器主要承担OS内核之外的一部分调度任务，他有自己的数据结构（简化）
```
type schedt struct {
    ...
    midle        muintptr //调度器空闲M链表
    pidle      puintptr   //调度器空闲P列表
    runq     gQueue      //调度器可运行g队列
    gFree struct{
        ...
    }       // 调度器自由G列表  
    gcwaiting  uint32  //表示gc正在等待运行
    stopwait   int32   //需要stop但仍未stop的p的数量
    sysmonwait uint32  //停止调度期间系统监控任务是否等待
    ...
}
```
* schedt结构体中的Lock是非常必须的，如果M或P等做一些非局部的操作，它们一般需要先锁住调度器。
* 几个重要的数据结构
    * 【1】调度器的空闲M列表：存放空闲M的一个单向链表调度器的
    * 【2】空闲P列表：存放空闲P的一个单向链表
    * 【3】调度器的可运行G队列：存放可运行G的队列
    * 【4】调度器的自由G队列：存放自由G的两个单向链表
* 调度器初始化schedinit，proc.go文件
```
func schedinit() {
    //getg是编译器函数，它会返回当前指针指向的g
    _g_ := getg()
    
    //调度器初始化伊始，会设置M的最大数量10000，意味着最多有1000 个M能够服务于当前go程序
    sched.maxmcount = 10000
    
    //其他的一些初始化函数
    ...
    
    //初始化栈空间复用管理链表
    stackinit()
    
    //内存分配初始化
    mallocinit()
    
    //初始化当前M
    mcommoninit(_g_.m)
    ...
    
    //p的默认值设置为cpu核数
    procs := ncpu
    if n, ok := atoi32(gogetenv("GOMAXPROCS")); ok && n > 0 {
    	procs = n
    }
    //调整P的数量，这个在上文提到过
    if procresize(procs) != nil {
        throw("unknown runnable goroutine during bootstrap")    
    }
    ...
}
```

## 4、go func()的执行过程（融会贯通）
* 在go语句执行中，编译器会将go语句转化为对newproc的调用，这会创建一个新的G
```
func newproc(siz int32, fn *funcval) {
    //获取第一参数地址
    argp := add(unsafe.Pointer(&fn), sys.PtrSize)
    
    //获取当前g的指针
    gp := getg()
    
    //获取调用方的PC程序计数器
    pc := getcallerpc()
    
    //从g0栈调用systemstack创建一个G对象，“更加高层的系统调用”
    systemstack(func() {
        newproc1(fn, argp, siz, gp, pc)
    })
}
```
* 上面的`systemstack()`是更加高层的系统调用
* `systemstack()`会切换当前的g到g0, 并且使用g0的栈空间, 然后调用传入的函数, 再切换回原来的g和原来的栈空间
```
func newproc1(fn *funcval, argp unsafe.Pointer, narg int32, callergp *g, callerpc uintptr) {
    // 调用getg获取当前的g, 会编译为读取FS寄存器(TLS), 这里会获取到g0
    _g_ := getg()
    ...
    
    _p_ := _g_.m.p.ptr() // 通过g0.m，获取m拥有的p
    newg := gfget(_p_) // 马上讲到
    
    // 如果从调度器和本地P的自由链表都获取不到G，就新建一个
    if newg == nil {
    	newg = malg(_StackMin) //新建g
        
        // 需要先设置g的状态为已中止(_Gdead), 这样gc不会去扫描这个g的未初始化的栈
        casgstatus(newg, _Gidle, _Gdead)
        
        // 将新建的G加入全局G列表中
    	allgadd(newg) // publishes with a g->status of  Gdead so GC scanner doesn't look at uninitialized stack.
    }

   
    // 1、把参数复制到g的栈上
    // 2、把返回地址复制到g的栈上, 这里的返回地址是goexit, 表示调用完目标函数后会调用goexit
    // 3、设置g的调度数据(sched)
    //  3-1设置sched.sp等于参数+返回地址后的rsp地址
    //  3-2设置sched.pc等于目标函数的地址, 查看gostartcallfn和gostartcall
    //  3-3设置sched.g等于g
    ...

    // 对G的一次初始化，无论G是新建还是获取到的
    // 初始化基本状态为Grunnable
    casgstatus(newg, _Gdead, _Grunnable)

    //将G放入P的可运行队列
    runqput(_p_, newg, true)

    // 1、如果当前有空闲的P
    // 2、但是无自旋的M(nmspinning等于0),
    // 3、并且主函数已执行则唤醒或新建一个M
    // 具体查看“空闲M链表”
    if atomic.Load(&sched.npidle) != 0 && atomic.Load(&sched.nmspinning) == 0 && mainStarted {
        //唤醒M，具体可看下文
        wakep()
    }
    releasem(_g_.m)

}
```
#### 4-1 gfget()
* 可以看下gfget是如何从当前P的自由G链表获取G的
```
func gfget(_p_ *p) *g {
retry:
    //如果当前P的自由G链表为空，尝试从调度器的自由g链表转移一部分P到本地
    // 调度器有两个自由链表（其实是栈）：1、stack；2、noStack
    if _p_.gFree.empty() && (!sched.gFree.stack.empty() ||  !sched.gFree.noStack.empty()) {
        lock(&sched.gFree.lock)
        // Move a batch of free Gs to the P.
        //最多转移32个
        for _p_.gFree.n < 32 {
            // Prefer Gs with stacks.
            gp := sched.gFree.stack.pop()
            if gp == nil {
                gp = sched.gFree.noStack.pop()
                if gp == nil {
                    break
                }
            }
            sched.gFree.n--
            _p_.gFree.push(gp) // 调度器->本地p
            _p_.gFree.n++
    	}
    	unlock(&sched.gFree.lock)
    	goto retry
    }
    // 如果当前P的自由G链表不为空，获取G对象
    // 首先调用gfget从p.gfree获取g, 如果之前有g被回收在这里就可以复用
    gp := _p_.gFree.pop()
    if gp == nil {
        return nil
    }
    //调整P的G链表
    _p_.gFree.n--
    //后续对G stack的一些处理   
    return gp  
}
```
* 当goroutine执行完毕，调度器会将G对象放回P的自由G链表，而不会销毁
#### 4-2 runqput()
* 在获取到G的时候，调度器会将其放入P的可运行G队列等待执行，proc.go文件
```
func runqput(_p_ *p, gp *g, next bool) {
    ...
    // 这个if有待进一步理解
    // 将G直接保存在本地P的runnext字段中
    // 首先随机把g放到p.runnext, 如果放到runnext则入队原来在runnext的g
    if next {
        retryNext:
    	oldnext := _p_.runnext
    	if !_p_.runnext.cas(oldnext, guintptr(unsafe.Pointe (gp))) {
            goto retryNext
        }
       	if oldnext == 0 {
            return
    	}
        // Kick the old runnext out to the regular run queue.
        // 原本的next G会被放回本地队列
        gp = oldnext.ptr()
    }
    retry:
    // P的可运行队列头
    h := atomic.LoadAcq(&_p_.runqhead) // load-acquire, synchronize with consumers
    
    // p的可运行队列尾
    t := _p_.runqtail
    
    //如果P的本地队列未满，直接放到尾部
    if t-h < uint32(len(_p_.runq)) {
    	_p_.runq[t%uint32(len(_p_.runq))].set(gp)
    	atomic.StoreRel(&_p_.runqtail, t+1) //  store-release, makes the item available for   consumption
    	return
    }
    
    //如果已满的话 会追加到调度器的可运行G队列
    if runqputslow(_p_, gp, h, t) {
    	return
    }

    // 上面两个追加操作都失败了，再来一遍
    // the queue is not full, now the put above must succeed
    goto retry
}
```
#### 4-3 runqputslow()
```
// 1、往调度器的队列添加任务，需要加锁 所以称为slow
// 2、会把本地运行队列中一半的g放到全局运行队列, 这样下次就可以继续用快速的本地运行队列了
func runqputslow(_p_ *p, gp *g, h, t uint32) bool {
    //将本地P的一半任务转移到调度器队列中
    var batch [len(_p_.runq)/2 + 1]*g 
    
    //调整计算过程
    ... 
    
    var q gQueue
    q.head.set(batch[0])
    q.tail.set(batch[n])    
    
    // Now put the batch on global queue.
    lock(&sched.lock)
    
    //添加到调度器的可运行G链表尾部
    globrunqputbatch(&q, int32(n+1))
    
    unlock(&sched.lock)
    return true
}

func globrunqputbatch(batch *gQueue, n int32) {
    sched.runq.pushBackAll(*batch)
    sched.runqsize += n
    *batch = gQueue{} 
}
```
#### 4-4 wakep()

* 唤醒或新建一个M会通过wakep函数
* 在newproc1成功创建G任务后 如果有空闲P 会尝试用wakep唤醒M执行任务
```
func wakep() {
    // 被唤醒的线程需要绑定P，累加自旋计数，避免newproc1唤醒过多线程
    // 首先交换nmspinning到1, 成功再继续, 多个线程同时执行wakep只有一个会继续
    if !atomic.Cas(&sched.nmspinning, 0, 1) {
        return
    }
    startm(nil, true)
}
```

```
func startm(_p_ *p, spinning bool) {
    ...
    //如果没有本地P，尝试获取空闲P(优先找本地的P)
    if _p_ == nil {
        // 调用pidleget从"空闲P链表"获取一个空闲的P
        _p_ = pidleget()
        if _p_ == nil { // 如果还是没有空闲的P，报错返回
            unlock(&sched.lock)
            if spinning {
                // The caller incremented nmspinning, but there are no idle Ps,
                // so it's okay to just undo the increment and give up.
                if int32(atomic.Xadd(&sched.nmspinning, -1)) < 0 {
                    throw("startm: negative nmspinning")
                }
            }
            return
        }
    }

    // 已经找到空闲的P了

    // 调用mget从"空闲M链表"获取一个空闲的M
    mp := mget()
    // 如果没有闲置的M，newm新建
    if mp == nil {
        var fn func()
        if spinning {
            // The caller incremented nmspinning, so set spinning in the new M.
            fn = mspinning
        }
        newm(fn, _p_)
        return
    }
    // 设置自旋状态
    mp.spinning = spinning
    // 临时存放P
    mp.nextp.set(_p_)
    // 调用notewakeup(&mp.park)唤醒线程
    notewakeup(&mp.park)
}
```
* M对象的创建前文中已经讲过，也就是说`startm`这个过程同样有两种方式，一种从调度器空闲M链表获取，一种新建一个M。
* **进入工作状态的M，会陷入调度循环，从各种可能的场所获取G也就是下文提到的全力查找可运行的G，一旦获取到G会立即运行这个G，前提是这个G未与其他M绑定。只有找不到可运行的G或者因为系统调用阻塞等原因被剥夺P，才会进入休眠状态。**


## 5、Go调度器调度过程（宏观）
* 【1】首先创建一个G对象，G对象保存到P本地队列或者是全局队列。P此时去唤醒/创建一个M。P继续执行它的执行序。
* 【2】唤醒/创建后的M寻找是否有空闲的P（这个过程通过g0实现），如果有则将该G对象移动到它本身。
>接下来M执行一个调度循环:
调用G对象->执行->清理线程→继续找新的Goroutine执行)。

* 【3】context switch
  * M执行过程中，随时会发生上下文切换。当发生上线文切换时，需要对执行现场进行保护，以便下次被调度执行时进行现场恢复。
  * Go调度器M的栈保存在G对象上，只需要将M所需要的寄存器(SP、PC等)保存到G对象上就可以实现现场保护。当这些寄存器数据被保护起来，就随时可以做上下文切换了，在中断之前把现场保存起来。
  * 如果此时G任务还没有执行完，M可以将任务重新丢到P的任务队列，等待下一次被调度执行。
    * 注意，接着M就是真正的阻塞了，不会继续在真正的CPU上面执行别的G，而是会使用hand off机制
    * hand off:当本线程因为 G 进行系统调用阻塞时，线程释放绑定的 P，把 P 转移给其他空闲的线程执行
  * 当再次被调度执行时，M通过访问G的vdsoSP、vdsoPC寄存器进行现场恢复(从上次中断位置继续执行)。
> 这里的上下文切换**任然是**内核级别的切换，而p调度器级别的切换**是挑选G绑到M上面运行**


## 6、Go调度器调度过程（源码实现）

#### 6-1 源码实现的大致思路

* 在一轮调度的开始，调度器会先判断当前M是否已经锁定
  * 如果发现当前M已经与某个G锁定，就会立即停止调度并停止当前M。一旦与他锁定的G处于可运行状态，他就唤醒M并继续运行那个G。
  * 如果当前M并未与任何G锁定，调度器会检查是否有运行时串行任务正在等待执行
    * 如果有，M会被停止并阻塞已等待运行时串行任务执行完成。一旦该串行任务执行完成，该M就会被唤醒。（有待深入学习go的GC）
* 调度器首先会从全局可运行G队列和本地P队列查找可运行的G
  * 如果找不到，调度器会进入强力查找模式，如果还找不到的话，该子流程就会暂停，直到有可运行的G的出现才会继续下去。
  * 如果找到可运行的G，调度器会判断该G未与任何M锁定的情况下，立即让当前M运行它。如果G已经锁定，那么调度器会唤醒与该G锁定的M并运行该G，停止当前M直到被唤醒。

#### 6-2 mstart

* M启动时会调用`mstart`函数, m0在初始化后调用, 其他的的m在线程启动后调用.`mstart`流程如下：
  * 调用getg获取当前的g, 这里会获取到g0
  * 如果g未分配栈则从当前的栈空间(系统栈空间)上分配, 也就是说g0会使用系统栈空间
  * 调用mstart1函数
```
func mstart1() {
    _g_ := getg() // g0

    // 调用gosave函数保存当前的状态到g0的调度数据中
    // 以后每次调度都会从这个栈地址开始
    // 简而言之，就是保存context
    gosave(&_g_.m.g0.sched)
    _g_.m.g0.sched.pc = ^uintptr(0)

    // 调用asminit函数, 不做任何事情
    asminit()

    // 调用minit函数, 设置当前线程可以接收的信号(signal)
    minit()

    if _g_.m == &m0 {
        initsig(false)
    }

    if fn := _g_.m.mstartfn; fn != nil {
        fn()
    }

    // 调用schedule函数
    schedule()
}
```
* 前言提到的M执行并发任务的起点的第二种方式是`stopm`休眠唤醒,从而进入调度，而这种方式的M也**仅是从断点状态恢复**，调度器判断M未锁定的话就进入**获取G的调度循环**中
#### 6-3 schedule

* **schedule中的动作大体就是找到一个等待运行的g，然后然后搬到m上，设置其状态为Grunning,直接切换到g的上下文环境,恢复g的执行**。进入`schedule`函数后，M就进入了核心调度循环。
* 大致流程：
  * **schedule函数获取g => [必要时休眠] => [唤醒后继续获取] => execute函数执行g => 执行后返回到goexit => 重新执行schedule函数**


* 简单来说g所经历的几个主要的过程就是：Gwaiting->Grunnable->Grunning。经历了创建,到挂在就绪队列,到从就绪队列拿出并运行整个过程。

```
func schedule() {
    // 调用getg获取当前的g和g所属的m实例
    _g_ := getg()

    ...

    // 如果发现当前M已经与某个G锁定，就会立即停止调度并停止当前M
    // 1、为什么要停止调度？因为schedu()目的是找到一个可以执行的G绑定到空的M上面运行。
    // 到目前为止，只是找到一个可以运行的空的M而已，而如果找到的M已经和某个G绑定了，自然不是空的M了
    // 2、既然M已经和G绑定了，那直接运行不就好了？因为要先停止对于目前M的调度，
    // 并且把他休眠，得到其所绑定的G回来后，才允许运行他
    if _g_.m.lockedg != 0 {

        // M已经和G绑定了，停止调度并停止当前M
        stoplockedm()

        // 一旦与他锁定的G处于可运行状态，他就唤醒M并继续运行那个G
        // Never returns.
        execute(_g_.m.lockedg.ptr(), false) 
    }

top: 
    // 开始核心调度循环
    ...
    pp := _g_.m.p.ptr()
    pp.preempt = false
    
    // gcwaiting表示当前需要停止M
    if sched.gcwaiting != 0 {
        //STW 停止阻塞当前M以等待运行时串行任务执行完成
        gcstopm()
        goto top
    }
    
    ...
   
    var gp *g
    var inheritTime bool

    ...

    // 以上代码，还需进一步学习才能更好的理解
    // 现阶段，重点掌握下面的内容
    
    // GC MarkWorker 工作模式
    // 试图获取执行GC标记任务的G
    if gp == nil && gcBlackenEnabled != 0 {
    	gp = gcController.findRunnableGCWorker(_g_.m    p.ptr())
    	tryWakeP = tryWakeP || gp != nil
    }
   
    // 每隔一段时间就去从全局队列获取G任务，确保公平性，为了公平起见, 每61次调度从全局运行队列获取一次G
    // 否则，两个goroutines会通过不断地彼此刷新来完全占用本地运行队列
    // 因为这两个goroutine可以循环一直占用时间片
    if gp == nil {
        // Check the global runnable queue once in a while to ensure fairness.
        // Otherwise two goroutines can completely occupy the local runqueue
        // by constantly respawning each other.
        if _g_.m.p.ptr().schedtick%61 == 0 && sched.runqsize > 0 {
            lock(&sched.lock)
            gp = globrunqget(_g_.m.p.ptr(), 1)
            unlock(&sched.lock)
        }
    }
    // 试图从本地P的可运行G队列获取G
    if gp == nil {
        gp, inheritTime = runqget(_g_.m.p.ptr())
    }

    // 快速获取失败时, 调用findrunnable函数获取待运行的G, 会阻塞到获取成功为止
    if gp == nil {
        // blocks until work is available
        gp, inheritTime = findrunnable()    
    }

    // 此时，成功获取到一个待运行的G
    
    // 1、让M离开自旋状态, 调用resetspinning, 这里的处理和findrunnable的不一样
    // 2、如果当前有空闲的P, 但是无自旋的M(nmspinning等于0), 则唤醒或新建一个M
    // 3、这里离开自选状态是为了执行G, 所以会检查是否有空闲的P, 有则表示可以再开新的M执行G
    // 4、findrunnable离开自旋状态是为了休眠M, 所以会再次检查所有队列然后休眠
    if _g_.m.spinning {
        resetspinning()
    }
    
    ...
    // 如果G要求回到指定的M(G和M已经绑定)，唤醒锁定的M运行该G
    if gp.lockedm != 0 {
        // Hands off own p to the locked m,
        // then blocks waiting for a new p.
        // 把G和P交给该M, 自己进入休眠
        startlockedm(gp)

        // 从休眠唤醒后跳到schedule的顶部重试
        goto top
    }

   // 执行goroutine任务函数
   execute(gp, inheritTime)
}
```

* 值得注意的是如果M被标记为自旋状态，意味着还没有找到G来运行，而无论是因为找到了可运行的G又或者因为始终未找到可运行的G而需要停止M，当前M都会退出自旋状态
* 提一点,一般情况下，运行时系统中至少会有一个自旋的M，调度器会尽量保证有一个自旋M的存在。除非没有自旋的M，调度器是不会新启用或回复一个M去运行新G的，一旦需要新启用一个M或者恢复一个M，他最初都是处于自旋状态。
* 整个一轮调度过程如表示：

#### 6-4 execute
```
func execute(gp *g, inheritTime bool) {
    // 调用getg获取当前的g
    _g_ := getg()
    _g_.m.curg = gp // gp是即将要执行的g，绑定到m上面
    gp.m = _g_.m
    
    ...
	
    // 把G的状态由待运行(_Grunnable)改为运行中(_Grunning)
    casgstatus(gp, _Grunnable, _Grunning)
    
    gp.waitsince = 0
    
    gp.preempt = false
    
    // 设置G的stackguard, 栈空间不足时可以扩张
    gp.stackguard0 = gp.stack.lo + _StackGuard

    // 这个函数会根据g.sched中保存的状态恢复各个寄存器的值并继续运行g
    gogo(&gp.sched)
}
```

#### 6-5 gogo
* 这个函数会根据`g.sched`中保存的状态恢复各个寄存器的值并继续运行g
  * 首先针对`g.sched.ctxt`调用**写屏障**(GC标记指针存活), `ctxt`中一般会保存指向[函数+参数]的指针
  * 设置TLS中的g为`g.sched.g`, 也就是g自身
  * 设置rsp寄存器为`g.sched.rsp`
  * 设置rax寄存器为`g.sched.ret`
  * 设置rdx寄存器为`g.sched.ctxt` (上下文)
  * 设置rbp寄存器为`g.sched.rbp`
  * 清空sched中保存的信息
  * 跳转到`g.sched.pc`，开始执行pc
* **因为前面创建`goroutine`的`newproc1`函数把返回地址设为了`goexit`, 函数运行完毕返回时将会调用`goexit`函数**
* `g.sched.pc`在G首次运行时会指向目标函数的第一条机器指令,
如果G被抢占或者等待资源而进入休眠, 在休眠前会保存状态到`g.sched`,`g.sched.pc`会变为唤醒后需要继续执行的地址, **保存状态**的实现将在下面讲解

#### 6-6 执行goexit前的保存状态操作
* 目标函数执行完毕后会调用`goexit`函数, `goexit`函数会调用`goexit1`函数, `goexit1`函数会通过`mcall`调用`goexit0`函数.
* `mcall`这个函数就是用于实现**保存状态**的, 处理如下:
    * 设置`g.sched.pc`等于当前的返回地址
    * 设置`g.sched.sp`等于寄存器rsp的值
    * 设置`g.sched.g`等于当前的g
    * 设置`g.sched.bp`等于寄存器rbp的值
    * **！！切换TLS中当前的g等于m.g0**
    * **！！设置寄存器rsp等于`g0.sched.sp`, 使用g0的栈空间**
    * 设置第一个参数为原来的g
    * 设置rdx寄存器为指向函数地址的指针(上下文)
    * 调用指定的函数, 不会返回
* `mcall`这个函数保存当前的运行状态到`g.sched`, 然后切换到g0和g0的栈空间, 再调用指定的函数
* 回到g0的栈空间这个步骤非常重要, 因为这个时候g已经中断, 继续使用g的栈空间且其他M唤醒了这个g将会产生灾难性的后果（**g的栈空间只属于他自己**）
* G在中断或者结束后都会通过`mcall`回到g0的栈空间继续调度, 从`goexit`调用的`mcall`的保存状态其实是多余的, 因为G已经结束了

#### 6-7 mcall保存状态之后的继续调用goexit0
* `goexit1`函数会通过`mcall`调用`goexit0`函数, `goexit0`函数调用时已经回到了**g0**的栈空间, 处理如下:
    * 把G的状态由运行中(`_Grunning`)改为已中止(`_Gdead`)
    * 清空G的成员
    * **调用`dropg`函数解除M和G之间的关联**
    * 调用gfput函数把**G放到P的自由列表中**, 下次创建G时可以复用
    * 调用`schedule`函数继续调度
    * G结束后回到`schedule`函数, 这样就结束了一个调度循环
* 不仅只有G结束会重新开始调度, **G被抢占或者等待资源**也会重新进行调度, 下面继续来看这两种情况


## 7、抢占式调度
#### 7-1 前言
* 当有很多`goroutine`需要执行的时候，是怎么调度的了，上面说的P还没有出场呢，在`runtime.main`中会创建一个额外m运行`sysmon`函数，抢占就是在`sysmon`中实现的。

* `sysmon`会进入一个无限循环, 第一轮回休眠20us, 之后每次休眠时间倍增, 最终每一轮都会休眠10ms. 
* `sysmon`中有
  * `netpool`获取fd事件
  * `retake`抢占
  * `forcegc`按时间强制执行gc
  * `scavenge heap`释放自由列表中多余的项减少内存占用
```
func sysmon() {
    lasttrace := int64(0)
    idle := 0 // how many cycles in succession we had not wokeup somebody
    delay := uint32(0)
    for {
        if idle == 0 { // start with 20us sleep...
            delay = 20
        } else if idle > 50 { // start doubling the sleep after 1ms...
            delay *= 2
        }
        if delay > 10*1000 { // up to 10ms
            delay = 10 * 1000
        }
        usleep(delay)
        ......
    }       
}
```
#### 7-2 retake
* 里面的函数`retake`负责抢占
```
func retake(now int64) uint32 {
    n := 0

    // 枚举所有的P
    for i := int32(0); i < gomaxprocs; i++ {
        _p_ := allp[i]
        if _p_ == nil {
            continue
        }
        pd := &_p_.sysmontick
        s := _p_.status

        if s == _Psyscall { // 执行系统调用
            // 如果p的syscall时间超过一个sysmon tick则抢占该p
            t := int64(_p_.syscalltick)
            if int64(pd.syscalltick) != t {
                pd.syscalltick = uint32(t)
                pd.syscallwhen = now
                continue
            }
            if runqempty(_p_) && atomic.Load(&sched.nmspinning)+atomic.Load(&sched.npidle) > 0 && pd.syscallwhen+10*1000*1000 > now {
                continue
            }
            incidlelocked(-1)
            if atomic.Cas(&_p_.status, s, _Pidle) {
                if trace.enabled {
                    traceGoSysBlock(_p_)
                    traceProcStop(_p_)
                }
                n++
                _p_.syscalltick++
                handoffp(_p_) // hand off 机制,解除关联
            }
            incidlelocked(1)
        } else if s == _Prunning { // 正在执行
            // 如果G运行时间过长，则抢占该G
            t := int64(_p_.schedtick)
            if int64(pd.schedtick) != t {
                pd.schedtick = uint32(t)
                pd.schedwhen = now
                continue
            }
            if pd.schedwhen+forcePreemptNS > now {
                continue
            }
            preemptone(_p_) // 执行抢占
        }
    }
    return uint32(n)
}
```
* `retake`函数负责处理抢占, 流程是:枚举所有的P
  * 如果P在系统调用中(`_Psyscall`), 且经过了一次sysmon循环(20us~10ms), 则抢占这个P，调用handoffp解除M和P之间的关联
  * 如果P在运行中(`_Prunning`), 且经过了一次sysmon循环并且G运行时间超过`forcePreemptNS`(10ms), 则抢占这个P，调用`preemptone`函数
    * 设置`g.preempt = true`
    * 设置`g.stackguard0 = stackPreempt`
    * 判断可以抢占之后，保存环境，执行`gopreempt_m`

#### 7-3 gopreempt_m
* `gopreempt_m`函数会调用`goschedImpl`函数, `goschedImpl`函数的流程是:
  * 把G的状态由运行中(`_Grunnable`)改为待运行(`_Grunnable`)
  * **调用`dropg`函数解除M和G之间的关联**
  * 调用`globrunqput`把G放到全局运行队列
  * 调用`schedule`函数继续调度
* 因为全局运行队列的优先度比较低, 各个M会经过一段时间再去重新获取这个G执行,抢占机制保证了不会有一个G长时间的运行导致其他G无法运行的情况发生.

#### 7-4 如何判断抢占
* 为什么设置了`stackguard`就可以实现抢占?
    * 因为这个值用于检查当前栈空间是否足够, go函数的开头会比对这个值判断是否需要**扩张栈**，具体实现如下
* `stackPreempt`是一个特殊的常量, 它的值会比任何的栈地址都要大, 检查时一定会触发栈扩张（空间一定不足，申请栈扩张）
    * 【1】栈扩张调用的是`morestack_noctxt`函数,` morestack_noctxt`函数清空`rdx`寄存器并调用`morestack`函数，`morestack`函数会保存G的状态到`g.sched`, **切换到g0和g0的栈空间**, 然后调用`newstack`函数.
    * 【2】`newstack`函数判断`g.stackguard0`等于`stackPreempt`, **就知道这是抢占触发的**, 这时会再检查一遍是否要抢占:
      * 如果M被锁定(函数的本地变量中有P), 则跳过这一次的抢占并调用`gogo`函数继续运行G
      * 如果M正在分配内存, 则跳过这一次的抢占并调用`gogo`函数继续运行G
      * 如果M设置了当前不能抢占, 则跳过这一次的抢占并调用`gogo`函数继续运行G
      * 如果M的状态不是运行中, 则跳过这一次的抢占并调用`gogo`函数继续运行G
    > 并发环境下，M状态也在变化，真正要抢占M时，M已经不可以抢占了
    * 【3】即使这一次抢占失败, **因为`g.preempt`等于`true`**, `runtime`中的一些代码会重新设置`stackPreempt`以重试下一次的抢占.
    * 【4】如果判断可以抢占, 则继续判断是否GC引起的,
      * 如果是,则对G的栈空间执行标记处理(扫描根对象)然后继续运行,
      * 如果不是GC引起的则调用`gopreempt_m`函数完成抢占.

## 8、再次理解m0、g0

* 在运行时，每个M都会有一个特殊的G，一般称为M的g0。g0是一个默认8KB栈内存的G，他的栈内存地址被传给`newosproc`函数，作为系统线程默认的堆栈空间。在之前M创建的时候，我们看到每个M创建之初都会调用`malg`创建一个`g0`。也就是说每个g0都是运行时系统在初始化M是创建并分配给M的。
* M的g0一般用于**执行调度、垃圾回收、栈管理**等方面的任务。除了g0外，其他M运行的G都可以视为**用户级别的G**。
* 除了每个M都有属于他的g0外，还存在一个runtime.g0。这个g0用于**执行引导程序**，它运行在Go程序拥有的第一个内核线程中，这个内核线程也被称为runtime.m0。

## 9、总结
* 相比大多数并行设计模型，Go比较优势的设计就是**P上下文这个概念的出现**，如果只有G和M的对应关系，那么当G阻塞在IO上的时候，M是没有实际在CPU工作的，这样造成了资源的浪费，没有了P，那么所有G的列表都放在全局，这样导致临界区太大，对多核调度造成极大影响
* 有了P之后，逻辑上切换多个G到多个M上面
    * 线程：涉及模式切换(从用户态切换到内核态)、16个寄存器、PC、SP...等寄存器的刷新等。
    * G：仅仅涉及g-g0的切换，且只有三个寄存器的值修改 - PC / SP / DX.
    * 协程拥有的栈只有2K，线程有8M
* 那么不是还有涉及到M在CPU上面的切换吗？不是照样要消耗？
  * 我的理解m:n总比1：1消耗少吧，况且实际的场景m>>n的

* 所以说**保护现场的抢占式调度**和**G被阻塞后传递给其他m调用**的核心思想，使得goroutine的产生

## 10、回顾历史
#### 10-1 进程、线程 和 协程 之间概念的区别
* 对于 进程、线程，都是有内核进行调度，有 CPU 时间片的概念，进行**抢占式调度**（有多种调度算法）
8 对于协程(用户级线程)，这是对内核透明的，也就是系统并不知道有协程的存在，是完全由用户自己的程序进行调度的，因为是由用户程序自己控制，那么就很难像抢占式调度那样做到强制的 CPU 控制权切换到其他进程/线程，通常只能进行**协作式调度**，需要协程自己主动把控制权转让出去之后，其他协程才能被执行到。
#### 10-2 goroutine 和协程区别
* 本质上，goroutine 就是协程。 不同的是，Golang 在 runtime、系统调用等多方面对 goroutine 调度进行了封装和处理，当遇到长时间执行或者进行系统调用时，**会主动把当前 goroutine 的CPU (P) 转让出去**,对于程序员来书，又不仅仅是协作式调度了。让其他 goroutine 能被调度并执行，也就是 Golang 从语言层面支持了协程。
#### 10-3 协程的历史以及特点
* 由于协程是非抢占式的调度，无法实现公平的任务调用。也无法直接利用多核优势。因此，我们不能武断地说协程是比线程更高级的技术。
* 尽管，在任务调度上，协程是弱于线程的。但是在资源消耗上，协程则是极低的。一个线程的内存在 MB 级别，而协程只需要 KB 级别。而且线程的调度需要内核态与用户的频繁切入切出，资源消耗也不小。
* 我们把协程的基本特点归纳为：
  * 1. 协程调度机制无法实现公平调度
  * 2. 协程的资源开销是非常低的，一台普通的服务器就可以支持百万协程。
* 那么，近几年为何协程的概念可以大热。我认为一个特殊的场景使得协程能够广泛的发挥其优势，并且屏蔽掉了劣势 --> 网络编程。与一般的计算机程序相比，网络编程有其独有的特点。
  * 1. 高并发（每秒钟上千数万的单机访问量）
  * 2. Request/Response。程序生命期端（毫秒，秒级）
  * 3. 高IO，低计算（连接数据库，请求API）。