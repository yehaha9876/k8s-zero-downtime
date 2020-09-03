i## zero-downtime

服务上云能解决的问题就是服务零停机，那怎么做的零停机，其中的内部原理是什么样的？

## 1. 创建go web程序
```
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
        "syscall"
	"time"
)

func sayhelloName(w http.ResponseWriter, r *http.Request) {
	t := time.Now().Unix()
	log.Print("start ", t)
	time.Sleep(time.Duration(5) * time.Second)
	fmt.Fprintf(w, "Hello World!")
	log.Print("return ", t)
}

func readinessProbe(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Reday!")
}

func main() {
	time.Sleep(time.Duration(10) * time.Second)
	log.Print("Start server")
        var srv = &http.Server{Addr: ":9090"}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool)
        signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	http.HandleFunc("/", sayhelloName)
	http.HandleFunc("/ready", readinessProbe)
	go func() {
		<-sigs
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(10) * time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Println("Pre shutdown whit no sigl:", err)
                }
		close(done)
                log.Println("Graceful shutdown down ")
	}()

	err := srv.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Fatal("ListenAndServe: ", err)
	}
	<-done
}

```

程序分析：
我先重点看readinessProbe方法，当web能提供服务的时候readynes方法就能正常访问。

首先我们先了解容器的启动与关闭，容器启动


## 容器在k8s的启动：

当容器启动时一般不会立即能提供服务，在这时候是runing但非ready状态，这状态是通过deployment中readinessProbe进行检查
```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zero-downtime
  namespace: zero-downtime
spec:
  replicas: 2
  selector:
    matchLabels:
      app: zero-downtime
  template:
    metadata:
      labels:
        app: zero-downtime
    spec:
      containers:
      - image: harbor.gzky.com/zero-downtime/web:latest
        imagePullPolicy: Always
        name: zero-downtime
        ports:
        - name: http
          containerPort: 9090
        readinessProbe:
          httpGet:
            path: /ready
            port: 9090
          initialDelaySeconds: 10
          periodSeconds: 5
```
我们通过方法没5秒访问一次ready接口来确认pod的可用性，当方法200后认为可用，注意initialDelaySeconds是指初始化延迟时间。
这样当10秒以后每5秒检查一次web程序是否可用，如果可用pod转变成ready状态。

然后我看考虑的是更新的时候的更新策略

```
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
```
这个告诉k8s最小可容忍几个不能用，滚动更新时每次更新几个pod

## 容器在k8s的关闭
我们可以能疑问，pod是怎么关闭的，pod如何在service中被移除的？
pod停止过程中是否存在请求？


1. 首先我们先说理论

当我们减少deployment中pod的个数是，kubeapi会先调用preStop命令，然后发送TERM（SIGTERM）信号到pod，然后隔30s（默认）发送SIGKILL信号
这时候我们需要程序能接收term命令，或者我们能让prostop命令处理来实现graceful shutdown

上面例子中go语言实例的graceful shutdown，当到SIGTERM信号时会停止接收新的请求，并等等10秒让旧的请求完成。

2. 那我们的疑问是在这10秒的停止过程中是否有新的请求进来？
我们先了解service的停止原理，停止被出触发会同时做三件事
1). 删除service中endpoint
2). 删除kuber-proxy中的iptable
3). 开始停止pod  

一般情况下endpoint会立即删除，但如果endpoint删除慢了几毫米，有流量到正在删除的pod上，那么应用就不可用了，为了解决这个问题，在真正收到preStop
先sleep几秒，具体根据集群的规模。然后在触发真正的graceful shutdown命令，如下


```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zero-downtime
  namespace: zero-downtime
spec:
  replicas: 2
  selector:
    matchLabels:
      app: zero-downtime
  template:
    metadata:
      labels:
        app: zero-downtime
    spec:
      containers:
      - image: harbor.gzky.com/zero-downtime/web:latest
        imagePullPolicy: Always
        name: zero-downtime
        lifecycle:
          preStop:
            exec:
              command: ["sh", "-c", "sleep 2"]
        ports:
        - name: http
          containerPort: 9090
        readinessProbe:
          httpGet:
            path: /ready
            port: 9090
          initialDelaySeconds: 10
          periodSeconds: 5
```

然后有人可能问，正在请求的连接会因为网络原因正常返回吗？
答案是会的。

下面是一个测试：
1. 测试service是否能正常停止
2. 测试router是否能正常停止
3. 测试ingress能否正常停止

结论是都可以。



