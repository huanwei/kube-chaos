# README
kube-chaos是一个kubernetes平台的故障注入组件，使用iproute2实现恶劣网络环境的模拟。

注意：运行kube-chaos时，hostname必须与k8s上显示的node名一致，这一般是默认的，但一旦用户主动更改，则会导致kube-chaos运行出错，原因在于k8s没有pod定位自身所在pod的API，因此只能通过hostname来确定所处的Node。

## 文档目录

* **运行方式与原理**
* **流程图**
* **部署方式**
* **测试方式**
* **使用方式**
* **功能与参数说明**
* **数据结构**
* **对外接口**

## 运行方式与原理
目前kube-chaos的实现是，以Daemonset的方式在每个包含“kube-chaos=on”标签的Node上调度一个以hostnetwork和特权模式启动的Pod，该Pod从集群中找到有chaos标签的Pod，根据它们的Annotation上的参数，对其所属的虚拟网卡进行配置，实现恶劣网络环境的模拟。

配置网卡的方案具体为：

* 对于Pod的egress流量，进入Pod所属的虚拟网卡Calixxxxxxxxxxx上的ingress，在这里将其转发到Node的IFB0网卡，对IFB0配置规则进行处理后发回；
* 对于Pod的ingress流量，来自Pod所属的虚拟网卡Calixxxxxxxxxxx上的egress，在这里将其转发到Node的IFB1网卡，对IFB1配置规则进行处理后发回；
* 在IFB网卡中针对各个Pod分类，将流量导到对应子类，在子类上挂载Netem队列，再发送回原处，实现对各个Pod的流量的分别控制。

### 网卡设置示意图
![](img/interface.png)


## 流程图

### 用户使用流程
![](img/userProcess.png)

### chaos执行流程
![](img/execProcess.png)

### TC队列配置示意图
![](img/TC_qdisc.png)

## 部署方式

### 项目编译
kube-chaos使用go语言编写，安装go编译工具后，在项目根目录中使用
`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -v -i -o kube-chaos kube-chaos.go`

生成kube-chaos的linux可执行程序，由于kube-chaos使用Linux内核实现故障注入，因此其中目标平台必须为Linux。

### 生成镜像
在项目根目录中有一个Dockerfile，用于建立kube-chaos的容器镜像，我们已经将生成镜像所需的命令编写在脚本文件`autobuild.sh`中，运行`sh autobuild.sh`即可完成镜像的构建。

### 部署前准备
kube-chaos通过label识别要控制的Node和Pod，对于Node，需要为想要运行kube-chaos的Node增加`chaos=on`标签，这样kube-chaos才会在该Node上调度出一个执行chaos的Pod，对该Node上的Pod进行故障注入；同样的，对于需要被注入故障的Pod，需要为它们增加`chaos=on`标签。

在没有`chaos=on`标签的Node上的Pod，即使Pod有`chaos=on`的标签，它也不会被注入故障；同理，在有`chaos=on`标签的Node上的没有chaos=on标签的Pod也不会被注入故障。

这一步也可以在kube-chaos启动后进行。

### 进行部署
kube-chaos以Daemonset的方式部署，部署配置在项目根目录中的chaos-daemonset.yaml中，在kube-chaos镜像生成后，使用kubectl根据该配置文件来部署kube-chaos:
`kubectl apply -f chaos-daemonset.yaml`

项目中的testpod目录下有一个autodeploy.sh文件，它包含了该条指令，执行`sh autodeploy.sh`效果相同。

### 停止故障注入
如果想要在停止kube-chaos后继续正常运行被注入的Pod，需要首先为这些Pod的annotation中增加`kubernetes.io/clear-ingress-chaos`或`kubernetes.io/clear-egress-chaos`标记，并等待该标记消失，此时针对该pod的ingress或egress故障注入配置将被清空，在所有被注入的Pod上完成该步骤后，可以将kube-chaos从集群中删除。

如果被注入的Pod也将同时关闭，则不需要上述步骤，直接在集群中删除kube-chaos即可。

在集群中删除kube-chaos，使用：`kubectl delete -f chaos-daemonset.yaml`即可。

如果需要停止特定Node的故障注入，需要为Node的annotation中增加`kubernetes.io/clear-chaos`标记，kube-chaos检测到该标记后会清理Node网络环境并删除Node的`chaos=on`标签，从而使kube-chaos不再在该Node上进行调度。

## 测试方式
kube-chaos提供了测试用的镜像和测试所需的脚本，你也可以使用自己的镜像用于测试。

在kube-chaos部署完毕后，在集群中运行测试用的镜像，并查看该产生的Pod的名字和IP地址用于测试。

> 如果要使用kube-chaos提供的测试镜像，使用testpod/目录下的autobuild.sh来创建测试镜像，并使用testpod.yaml来部署测试Pod。

获得测试Pod名和IP后，执行`sh /testpod/ingresstest.sh [PodName] [PodIP] >/dev/null &`来开启后台运行的自动测试，测试持续约2分钟，执行完毕后在`/tmp/test_output.txt`中查看测试结果。

测试脚本执行的测试内容为逐个为测试Pod注入各个类型的模拟网络环境，并对该Pod执行Ping来探测网络环境。

测试结果大致如下：

```
Kube-chaos TC egress test
 
Loss test: Percentage 50%,Relate 25% Rate limit 100kbps 
 
PING 192.168.102.234 (192.168.102.234) 56(84) bytes of data.
64 bytes from 192.168.102.234: icmp_seq=1 ttl=63 time=0.377 ms
64 bytes from 192.168.102.234: icmp_seq=2 ttl=63 time=0.343 ms
64 bytes from 192.168.102.234: icmp_seq=5 ttl=63 time=0.250 ms
64 bytes from 192.168.102.234: icmp_seq=6 ttl=63 time=0.316 ms
64 bytes from 192.168.102.234: icmp_seq=10 ttl=63 time=0.443 ms
64 bytes from 192.168.102.234: icmp_seq=11 ttl=63 time=0.412 ms
64 bytes from 192.168.102.234: icmp_seq=17 ttl=63 time=0.451 ms
64 bytes from 192.168.102.234: icmp_seq=21 ttl=63 time=0.451 ms
64 bytes from 192.168.102.234: icmp_seq=27 ttl=63 time=0.548 ms
64 bytes from 192.168.102.234: icmp_seq=28 ttl=63 time=0.464 ms
64 bytes from 192.168.102.234: icmp_seq=29 ttl=63 time=0.458 ms
64 bytes from 192.168.102.234: icmp_seq=30 ttl=63 time=0.389 ms
64 bytes from 192.168.102.234: icmp_seq=31 ttl=63 time=0.402 ms
64 bytes from 192.168.102.234: icmp_seq=32 ttl=63 time=0.527 ms
64 bytes from 192.168.102.234: icmp_seq=35 ttl=63 time=0.620 ms
64 bytes from 192.168.102.234: icmp_seq=40 ttl=63 time=0.442 ms
64 bytes from 192.168.102.234: icmp_seq=41 ttl=63 time=0.403 ms
64 bytes from 192.168.102.234: icmp_seq=42 ttl=63 time=0.567 ms
64 bytes from 192.168.102.234: icmp_seq=44 ttl=63 time=0.686 ms
64 bytes from 192.168.102.234: icmp_seq=45 ttl=63 time=0.289 ms

--- 192.168.102.234 ping statistics ---
45 packets transmitted, 20 received, 55% packet loss, time 608ms
rtt min/avg/max/mdev = 0.250/0.441/0.686/0.109 ms

...
```
观察测试的类型和参数并且观察ping的结果，可以用于确认kube-chaos是否正常运行并注入故障。

## 使用方式
目前完成的部分是最底层的执行组件，还没有自动执行的策略，因此需要手动用kubectl指定被测试的应用的所有pod的模拟参数，注意在命令行中需要为`"`符号前增加`\`转义符，例如在ingress方向加入延迟：

```
kubectl annotate pod $1 kubernetes.io/ingress-chaos="100kbps,delay,100ms,50ms" kubernetes.io/done-ingress-chaos=no --overwrite
```

并且要注意的是，要设置kubernetes.io/done-ingress-chaos=no以使设置生效，egress方向的设置类似。

## 功能与参数说明
### 输入
* pod的annotation上标注的chaos设置；
* pod的label（用于标记需要进行故障注入的pod）。

---

### 输出
* 应用chaos设置后chaos将改变annotation上的`kubernetes.io/done-ingress-chaos`字段和`kubernetes.io/done-egress-chaos`字段；
* 应用chaos设置后对应pod的网卡设置将会根据参数改变。

---

### 可注入故障类型

* **限速（Rate）**
* **延迟（Delay）** 
* **丢包（Loss）**
* **重复（Duplicate）**
* **乱序（Reorder）**
* **损坏（Corrupt）**

----------------------------
#### 限速
参数样例：`10kbps`

效果：限制带宽上限到10KB/s，需要注意的是，限速的上限为4gibps,不支持更高的限速（由于内核中的限速速率由一个单位为bits/s的32位的无符号整数来储存）。

可使用速率单位：
				
	bit or a bare number	Bits per second
	kbit	Kilobits per second
	mbit	Megabits per second
	gbit	Gigabits per second
	tbit	Terabits per second
	bps		Bytes per second
	kbps	Kilobytes per second
	mbps	Megabytes per second
	gbps	Gigabytes per second
>如果用IEC单位表示,则将SI的前缀（k-, m-, g-)替换为IEC的前缀(ki-, mi-, gi-)。

>另外也可以用一个百分数比如`50%`来表示占当前设备速率的百分比。

---
#### 延迟
参数样例：`,delay,10ms`

效果：产生一个平均为100ms，误差正负10ms的延迟。注意：参数首部的逗号不能忽略，第一个参数固定为限速速率，速率为空意味着最高速率，可以认为是限速到4gibps

可使用时间单位：

	s, sec or secs						Whole seconds
	ms, msec or msecs					Milliseconds
	us, usec, usecs or a bare number	Microseconds.
---
#### 丢包
参数样例：`,loss,50%,25%`

效果：产生50%的丢包几率，并且这个几率受到伪相关系数影响，即`下一次丢包几率=这次是否丢包*25%+50%*(1-25%)`。

---
#### 重复
参数样例：`,duplicate,50%`

效果：产生50%的重复包发生率，表现为大约每三个数据包中就有两个是一模一样的数据包。

---
#### 乱序
参数样例：`,delay,50ms,reorder,50%,25%`

效果：50%的数据包会产生50ms的延迟，从而导致包的顺序会被打乱，其中50%的几率受到25%的伪相关系数影响。

---
#### 损坏
参数样例：`,corrupt,3%`

效果：3%的数据包中会出现数据损坏（即数据被改变）。

## 数据结构
### TC控制参数
kube-chaos通过Pod上的Annotation进行网络环境模拟的配置。

样例：
`100kbps,delay,100ms,10ms`(该设置将网卡延迟增加100ms，误差10ms,最高带宽100kbps)。

### 参数更新标志
由于chaos通过annotation来进行设置，因此需要轮询各个pod的annotation，为此需要设置`kubernetes.io/done-ingress-chaos`或`kubernetes.io/done-egress-chaos`标志来指示设置的状态。

* 当新增或更改设置时，将`kubernetes.io/done-ingress-chaos`或`kubernetes.io/done-egress-chaos`标设置为no；
* 当chaos组件检测到`kubernetes.io/done-ingress-chaos`或`kubernetes.io/done-egress-chaos`标为no时将更新对应方向的设置，并在完成后将对应标志置为yes；
* 当chaos组件检测到`kubernetes.io/done-ingress-chaos`或`kubernetes.io/done-egress-chaos`为yes时，跳过对应方向的当前设置。

### 参数清空标志
当一个Pod需要恢复正常的网络环境时，单纯的删除参数无法达到效果，为了完成网络状态的恢复，需要设置`kubernetes.io/clear-ingress-chaos`或`kubernetes.io/clear-egress-chaos`来撤除kube-chaos对该Pod的网络设置，这项参数不需要设置值，只需要存在该键即可。

在kube-chaos完成ingress的恢复后，会将`kubernetes.io/clear-ingress-chaos`,`kubernetes.io/done-ingress-chaos`和`kubernetes.io/ingress-chaos`三个同方向的标志全部清空，egress同理。

## 对外接口
### Labels
label在chaos中起到选择对象的作用，对应用而言，使用label可以选择被注入故障的Pod，对集群而言，使用label可以选择注入故障所用的Node

#### chaos=on
默认设置中，对Node增加`chaos=on`标签可以使该Node拥有故障注入能力（即该Node上会被调度一个chaosPod），对Pod增加`chaos=on`标签可以使该Pod被chaos注入故障（前提是该Pod所在的Node拥有`chaos=on`标签

同时，标签名可以不是`chaos=on`，这可以通过设置chaos的`labelSelector`参数来改变

### Annotations
chaos依赖annotation提供参数进行故障注入，以下是可用的annotation列表：

#### kubernetes.io/ingress-chaos
这个参数用于设置Pod入境流量的故障注入参数，参数形式在文档数据结构部分和可注入故障类型中有详细描述，这里不再赘述

#### kubernetes.io/egress-chaos
这个参数用于设置Pod出境流量的故障注入参数

#### kubernetes.io/clear-ingress-chaos
本参数用于清除已经执行的Pod入境流量设置，如需在保持Pod运行状态的同时撤除chaos的故障注入，请设置本参数，chaos将会在一个更新周期内清除原设置并同时清除有关annotation。这项参数不需要设置值，只需要存在该键即可。

#### kubernetes.io/clear-egress-chaos
同上，本参数用于清除已经执行的Pod出境流量设置

#### kubernetes.io/done-ingress-chaos
本参数用于指示chaos进行入境流量故障注入的设置更新，当需要更新设置或清除设置时(包括`ingress-chaos`和`clear-ingress-chaos`)，将本参数设为`no`，chaos将会在一个更新周期内执行新的设置并将本参数设置为`yes`

#### kubernetes.io/done-ingress-chaos
同上，本参数用于指示chaos进行出境流量故障注入的设置更新

#### kubernetes.io/clear-chaos
本参数在Node上使用，用于指示chaos清理该node上的所有设置并关闭该node上的chaos，这个操作会导致node上的`chaos=on`标签被删除，chaosPod被关闭




