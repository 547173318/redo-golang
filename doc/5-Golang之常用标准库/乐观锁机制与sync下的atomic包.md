## 1、Go中的原子操作
* 原子性：一个或多个操作在CPU的执行过程中不被中断的特性，称为原子性。这些操作对外表现成一个不可分割的整体，他们要么都执行，要么都不执行，外界不会看到他们只执行到一半的状态。

* 原子操作：进行过程中不能被中断的操作，原子操作由底层硬件支持，而锁则是由操作系统提供的API实现，若实现相同的功能，前者通常会更有效率

* 最小案例：
```
package main

import (
	"sync"
	"fmt"
)

var count int

func add(wg *sync.WaitGroup) {
	defer wg.Done()
	count++
}

func main() {
	wg := sync.WaitGroup{}
	wg.Add(1000)
	for i := 0; i < 1000; i++ {
		go add(&wg)
	}
	wg.Wait()
	fmt.Println(count)
}

```
* count不会等于1000，因为count++这一步实际是三个操作：
    * 从内存读取count
    * CPU更新count = count + 1
    * 写入count到内存
* 因此就会出现多个goroutine读取到相同的数值，然后更新同样的数值到内存，导致最终结果比预期少

## 2、Go中sync/atomic包
* Go语言提供的原子操作都是非入侵式的，由标准库中sync/aotomic中的众多函数代表

* atomic包中支持六种类型
>int32 uint32 int64 uint64 uintptr unsafe.Pointer
* 对于每一种类型，提供了五类原子操作：

* LoadXXX(addr): 原子性的获取*addr的值，等价于：
```
return *addr      
```

* StoreXXX(addr, val): 原子性的将val的值保存到*addr，等价于：
```
*addr = val 
```

* AddXXX(addr, delta):
原子性的将delta的值添加到*addr并返回新值（unsafe.Pointer不支持），等价于：
```
*addr += delta
return *addr
```

* SwapXXX(addr, new) old: 原子性的将new的值保存到*addr并返回旧值，等价于：
```
old = *addr
*addr = new
return old
```

* CompareAndSwapXXX(addr, old, new) bool:
原子性的比较addr和old，如果相同则将new赋值给addr并返回true，等价于：
```
if *addr == old {
    *addr = new
    return true
}
return false
```

* 在这里插入图片描述,因此第一部分的案例可以修改如下，即可通过
```
// 修改方式1
func add(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		if atomic.CompareAndSwapInt32(&count, count, count+1) {
			break
		}
	}
}
// 修改方式2
func add(wg *sync.WaitGroup) {
	defer wg.Done()
	atomic.AddInt32(&count, 1)
}
```

## 3、扩大原子操作的适用范围：atomic.Value
#### 3-1 介绍
* Go语言在1.4版本的时候向sync/atomic包中添加了新的类型Value，此类型相当于一个容器，被用来"原子地"存储（Store）和加载任意类型的值

* type Value func(v *Value) Load() (x interface{}): 读操作，从线程安全的v中读取上一步存放的内容 
* func(v *Value) Store(x interface{}): 写操作，将原始的变量x存放在atomic.Value类型的v中
```
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
    // 此处依旧选用简单的数据类型，因为代码量少
    config := atomic.Value{}
    config.Store(22)    
    wg := sync.WaitGroup{}
    wg.Add(10)
    for i := 0; i < 10; i++ {
        go func(i int) {
            defer wg.Done()
            // 在某一个goroutine中修改配置
            if i == 0 {
            	config.Store(23)
            }
            // 输出中夹杂22，23
            fmt.Println(config.Load())
        }(i)
    }
    wg.Wait()
}
```
#### 3-2 unsafe.Pointer
* Go语言并不支持直接操作内存，但是它的标准库提供一种不保证向后兼容的指针类型unsafe.Pointer， 让程序可以灵活的操作内存，它的特别之处在于：可以绕过Go语言类型系统的检查

* 也就是说：如果两种类型具有相同的内存结构，我们可以将unsafe.Pointer当作桥梁，让这两种类型的指针相互转换，从而实现同一份内存拥有两种解读方式

* 例如int类型和int32类型内部的存储结构是一致的，但是对于指针类型的转换需要这么做：
```
var a int32
// 获得a的*int类型指针
(*int)(unsafe.Pointer(&a))
```
#### 3-3 atomic.Value源码分析
* atomic.Value被设计用来存储任意类型的数据，所以它内部的字段是一个interface{}类型
```
type Value struct {
    v interface{}
}
```
* 还有一个ifaceWords类型，作为空interface的内部表示格式，typ代表原始类型，data代表真正的值
```
// ifaceWords is interface{} internal representation.
type ifaceWords struct {
    typ  unsafe.Pointer
    data unsafe.Pointer
}
```

#### 3-4 实现原子性的读取任意结构操作
```
func (v *Value) Load() (x interface{}) {
    
    // 将*Value指针类型转换为*ifaceWords指针类型
    vp := (*ifaceWords)(unsafe.Pointer(v))
	
    // 原子性的获取到v的类型typ的指针
    typ := LoadPointer(&vp.typ)
    
    // 如果没有写入或者正在写入，先返回
    // ^uintpt(0)代表过渡状态，见下文
    if typ == nil || uintptr(typ) == ^uintpt(0) {
    	return nil
    }
    
    // 原子性的获取到v的真正的值data的指针，然后回
    data := LoadPointer(&vp.data)
    
    xp := (*ifaceWords)(unsafe.Pointer(&x))
    xp.typ = typ
    xp.data = data
    return
}
```

#### 3-5 实现原子性的存储任意结构操作
* 在此之前有一段较为重要的代码，其中runtime_procPin方法可以将一个goroutine死死占用当前使用的P (此处参考Goroutine调度器(一)：P、M、G关系, 不发散了) 不允许其他的goroutine抢占，而runtime_procUnpin则是释放方法
```
// Disable/enable preemption, implemented in runtime.
func runtime_procPin()
func runtime_procUnpin()
```

* Store方法
```
func (v *Value) Store(x interface{}) {
    if x == nil {
    	panic("sync/atomic: store of nil value  into Value")
    }
    // 将现有的值和要写入的值转换为ifaceWords类型   这样下一步就能获取到它们的原始类型和真正的值
    vp := (*ifaceWords)(unsafe.Pointer(v))
    xp := (*ifaceWords)(unsafe.Pointer(&x))
    for {
        // 获取现有的值的type
        typ := LoadPointer(&vp.typ)
        
        // 如果typ为nil说明这是第一次Store
        if typ == nil {
            
            // 如果你是第一次，就死死占住当前的processor，不允许其他goroutine再抢
            runtime_procPin()
			
            // 使用CAS操作，先尝试将typ设置为^uintptr(0)这个中间状态
            // 如果失败，则证明已经有别的线程抢先完成了赋值操作
            // 那它就解除抢占锁，然后重新回到 for 循环第一步
            if !CompareAndSwapPointer(&vp.typ, nil, unsafe.Pointer(^uintptr(0))) {
                runtime_procUnpin()
                continue
            }
            
            // 如果设置成功，说明当前goroutine中了jackpot
            // 那么就原子性的更新对应的指针，最后解除抢占锁
            StorePointer(&vp.data, xp.data)
            StorePointer(&vp.typ, xp.typ)
            runtime_procUnpin()
            return
        }

        // 如果typ为^uintptr(0)说明第一次写入还没有完成，继续循环等待
        if uintptr(typ) == ^uintptr(0) {
            continue
        }
        // 如果要写入的类型和现有的类型不一致，则panic
        if typ != xp.typ {
            panic("sync/atomic: store of inconsistently typed value into Value")
        }
        // 更新data
        StorePointer(&vp.data, xp.data)
        return
	}
}
```
