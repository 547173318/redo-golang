## 1、Snowflake算法源码解析
#### 1-1 结构体 
```
// A Node struct holds the basic information needed for a snowflake generator
// node
type Node struct {
	mu    sync.Mutex
	epoch time.Time
	time  int64
	node  int64
	step  int64

	nodeMax   int64
	nodeMask  int64
	stepMask  int64
	timeShift uint8
	nodeShift uint8
}
```

#### 1-2 构造函数
```
// An ID is a custom type used for a snowflake ID.  This is used so we can
// attach methods onto the ID.
type ID int64

// NewNode returns a new snowflake node that can be used to generate snowflake
// IDs
func NewNode(node int64) (*Node, error) {

	// re-calc in case custom NodeBits or StepBits were set
	// DEPRECATED: the below block will be removed in a future release.
	mu.Lock()
	nodeMax = -1 ^ (-1 << NodeBits)
	nodeMask = nodeMax << StepBits
	stepMask = -1 ^ (-1 << StepBits)
	timeShift = NodeBits + StepBits
	nodeShift = StepBits
	mu.Unlock()

	n := Node{}
	n.node = node
	n.nodeMax = -1 ^ (-1 << NodeBits)
	n.nodeMask = n.nodeMax << StepBits
	n.stepMask = -1 ^ (-1 << StepBits)
	n.timeShift = NodeBits + StepBits
	n.nodeShift = StepBits

	if n.node < 0 || n.node > n.nodeMax {
		return nil, errors.New("Node number must be between 0 and " + strconv.FormatInt(n.nodeMax, 10))
	}

	var curTime = time.Now()
	// add time.Duration to curTime to make sure we use the monotonic clock if available
	n.epoch = curTime.Add(time.Unix(Epoch/1000, (Epoch%1000)*1000000).Sub(curTime))

	return &n, nil
}
```
#### 1-3 Generate
```
// Generate creates and returns a unique snowflake ID
// To help guarantee uniqueness
// - Make sure your system is keeping accurate system time
// - Make sure you never have multiple nodes running with the same node ID
func (n *Node) Generate() ID {

	n.mu.Lock()

	now := time.Since(n.epoch).Nanoseconds() / 1000000

    // 如果同属于1ms,递增序号
	if now == n.time {

		// stepMask = 0000000000...0000000111111111111(12位1)
		n.step = (n.step + 1) & n.stepMask
	
		// 1ms内的递增序号已经使用完毕
		if n.step == 0 {

			// 阻塞等到新的1ms
			for now <= n.time {
				now = time.Since(n.epoch).Nanoseconds() / 1000000
			}
		}
	} else {
		// 已经是新的1ms了，递增序号清零
		n.step = 0
	}

	n.time = now

	r := ID((now)<<n.timeShift |   // 高42
		(n.node << n.nodeShift) |  // 10
		(n.step),                  // 低位12
	)

	n.mu.Unlock()
	return r
}
```
#### 1-4 总结
* 首先，时间戳随着系统时间增长，当时间戳的第42位增长到1时，ID的最高位也将变为1。由于snowflake默认使用有符号64位整型，最高位为符号位，这将导致生成的ID变为负数。通过如下公式(1<<41) / 1000 / 60 / 60 / 24 / 365计算，可知道在基准时间上（snowflake默认的基准时间是2010/11/4）再过69.7年，将出现ID为负数的情况。

>这里展开说明下，如果想要生成的ID永远不为负数，可以保持ID的最高位始终为0，其他的字段减少1位，比如说时间戳只使用41位。这样时间戳出现翻转归零的时长缩短1倍，大概为35年，基本上是可接受的。你可能会说，69.7年已经足够长啦，到出现负数的时候我早退休了，哪管它洪水滔天。但是假设你要设计一个32位的分布式ID生成器呢？此时你必然需要考虑哪些字段可以缩短，时间戳多久出现翻转（也即ID可能翻转）不影响业务。

* 第二，假如单台机器上，获取**当前时间的方法出现时间回退**，那么可能出现ID重复的情况。(或者更改基准时间导致两者差值重复)

* 第三，假如服务重启，重启后时间戳没变（即1毫秒内重启成功），那么此时snowflake丢失了重启前当前时间戳的递增序号，递增序号重新从0开始，也可能出现和重启前生成的ID重复的情况