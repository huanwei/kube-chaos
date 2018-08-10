# kube-chaos中使用的命令

ifb0与ifb1配置完全相同，可自行替换。

本文档中ifb0负责calico网卡的ingress流量，ifb1负责calico网卡的egress流量。

### 初始化ifb模块

` modprobe ifb`

`ip link set dev ifb0 up `

### 配置ifb队列

清空ifb上所有的队列

` tc qdisc del dev ifb0 root`

添加htb队列，handle为1，默认子类为0

`tc qdisc add dev ifb0 root handle 1: htb default 0`

### 配置calico网卡根队列

清空calico上所有的队列

`tc qdisc del dev calixxxxxxxxxxx root`

添加htb队列， handle为1，默认子类为1

`tc qdisc add dev calixxxxxxxxxxx root handle 1: htb default 1`

添加htb分类，parent为1，classid为1:1，最大速度为4gbps

`tc class add dev calixxxxxxxxxxx parent 1: classid 1:1 htb rate 4gbps`1

为htb分类添加pfifo队列，handle为2:1，队列长度1600

`tc qdisc add dev calixxxxxxxxxxx parent 1:1 handle 2:1 pfifo limit 1600`

### 配置calico网卡ingress队列

获取当前队列信息

`tc qdisc show dev calixxxxxxxxxxx ingress`

添加ingress队列

`tc qdisc add dev calixxxxxxxxxxx ingress`

### 为calico网卡配置htb分类

获取ifb当前所有分类，找出空闲类号

`tc class show dev ifb0`

为当前calico网卡新建分类

` tc class add dev ifb0 parent 1: classid 1:X htb rate XXX`

### 配置calico网卡的流量转发

##### pod的ingress（calico的egress）

添加流量转发规则

`tc filter add dev calixxxxxxxxxxx parent 1: protocol ip prio 1 u32 match\`

` u32 0 0 flowid 1:1 action mirred egress redirect dev ifb1`

添加ifb1上的过滤分类规则，ip地址为pod的ip，flowid为新建分类时保存的classid

`tc filter add dev ifb1 parent 1:0 protocol ip prio 1 u32 match ip dst xxx.xxx.xxx.xxx flowid 1:X` 

##### pod的egress（calico的ingress）

添加流量转发规则

`tc filter add dev calixxxxxxxxxxx parent ffff: protocol ip prio 1 u32 match\`

` u32 0 0 flowid 1:1 action mirred egress redirect dev ifb0`

添加ifb0上的过滤分类规则，ip地址为pod的ip，flowid为新建分类时保存的classid

`tc filter add dev ifb0 parent 1:0 protocol ip prio 1 u32 match ip src xxx.xxx.xxx.xxx flowid 1:X` 

### 删除netem队列

使用获取的类号作为parent编号，删除对应的队列

`tc qdisc del dev ifb0 parent 1:X`

### 初始化netem队列

使用获取的类号作为parent编号，添加空的netem队列

`tc qdisc add dev ifb0 parent 1:X netem`

### 添加netem过滤规则

delay，通过classid获取队列，两个参数分别是延迟时间和延迟时间波动范围

`tc qdisc change dev ifb0 parent 1:X netem delay $time $deviation` 

duplicate，参数为重发百分比

`tc qdsic change dev ifb0 parent 1:X netem duplicate $percentage`

reorder，参数为延迟时间，乱序率和相关度

`tc qdsic change dev ifb0 parent 1:X netem delay $time reorder $percentage $relate`

corrupt，参数为损坏百分比

`tc qdsic change dev ifb0 parent 1:X netem corrupt $percentage`