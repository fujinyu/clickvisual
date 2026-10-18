# iLogtail 配置参考

[官方文档](https://ilogtail.gitbook.io/ilogtail-docs/about/readme)


输出使用 [kafkaV2](https://ilogtail.gitbook.io/ilogtail-docs/data-pipeline/flusher/flusher-kafka_v2) 将日志数据发送至 Kafka

日志->kafka->clickhouse->clickviusal 的流程

## 具体流程

ilogtail configmap 目前只支持容器标准输出【service_docker_stdout】
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
name: ilogtail-user-cm
namespace: ilogtail
data:
    config.yaml: |
        enable: true
        inputs:
            - Type: service_docker_stdout
        Stderr: true
        Stdout: true
        flushers:
            - Type: flusher_kafka_v2
        Brokers:
            - 192.XX.XX.1:9092
                  - 192.XX.XX.2:9092
                  - 192.XX.XX.3:9092
        Topic: KafkaTestTopic
```

写入 kafka 应该是如下数据结构
```json 
{
    "contents": {
        "_source_": "stderr",
        "_time_": "2023-04-17T04:07:17.624075074Z",
        "content": "{\"lv\":\"debug\",\"ts\":1681704437,\"msg\":\"presigned get object URL\"}"
    },
    "tags": {
        "container.image.name": "xxx",
        "container.ip": "127.0.0.1",
        "container.name": "xx-xx",
        "host.ip": "127.0.0.1",
        "host.name": "xx-xx-xx",
        "k8s.namespace.name": "default",
        "k8s.node.ip": "127.0.0.1",
        "k8s.node.name": "127.0.0.1",
        "k8s.pod.name": "xx-xx-xx-xx",
        "k8s.pod.uid": "xx-xx-xx-xx-xx"
    },
    "time": 1681704438
}
```

在 clickvisual 中选择该方式创建日志库即可

![collect-table.png](../../../images/collect-table.png)

## 多行合并（一次异常 = 一条日志）

Java 异常堆栈会被容器运行时/采集器按行切开，默认一行一条日志，查询时同一次异常散成十几条，光靠排序键无法还原。两条采集链路的开关位置不同，正则引擎也不同。

### 容器标准输出链路（service_docker_stdout，Go 插件）

起始行判定在 `plugins/input/docker/stdout/docker_stdout_processor.go` 的 `Process()`：`BeginLineRegex` 非空即进入多行分支，匹配到起始行才开新块。正则是 Go `regexp`（RE2，**不支持负向前瞻**），而 `Caused by:`、`Suppressed:` 这类续行不带缩进、会被起始行正则误判成新块开头，因此本 fork 增加了 `ExcludeBeginRegex`：命中它的行即使匹配 `BeginLineRegex` 也并回上一块。

优先用时间戳锚定起始行（无需 `ExcludeBeginRegex`，现有镜像即可生效）：

```yaml
enable: true
inputs:
  - Type: service_docker_stdout
    Stderr: true
    Stdout: true
    IncludeK8sLabel:
      app: forest-cloud-modelservice-test
    BeginLineRegex: '^\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}'
    BeginLineCheckLength: 1024
    BeginLineTimeoutMs: 3000
    MaxLogSize: 524288
flushers:
  - Type: flusher_kafka_v2
    Brokers:
      - 172.16.20.21:9094
    Topic: test_forest-cloud-modelservice
    Version: "3.4.0"
    Convert:
      Protocol: custom_single_flatten
      Encoding: json
```

`BeginLineRegex` 里的格式串必须按该服务真实首行格式改，取证命令：

```bash
kubectl -n <ns> logs <pod> --tail=200 | head -40
```

首行没有时间戳时，改用"非缩进行即新块"+ 续行排除（**该写法依赖包含 `ExcludeBeginRegex` 的镜像，官方镜像无此字段**）：

```yaml
    BeginLineRegex: '^[^\t]'
    ExcludeBeginRegex: '^(?:Caused by:|Suppressed:|\.\.\.\s\d+\s+more|no stack trace available)'
```

续行缩进是否为制表符，用 `cat -A` 确认（制表符显示为 `^I`，行尾显示为 `$`）：

```bash
kubectl -n <ns> logs <pod> --tail=2000 | cat -A | grep -n -B1 -A2 'Caused by' | head -60
```

字段说明：

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `BeginLineRegex` | 空 | 空则每行一条日志；非空进入多行合并 |
| `ExcludeBeginRegex` | 空 | 本 fork 新增，命中即视为上一块的续行 |
| `BeginLineCheckLength` | 10240 | 只拿行首这么长做匹配，调小到 1024 可省 CPU |
| `BeginLineTimeoutMs` | 3000 | 块末尾多久没新行就强制输出 |
| `MaxLogSize` | 524288 | 单块字节上限，超长堆栈会被强制切开 |

已知行为：合并后一条日志的 `_time_` 取该块**最后一行**的时间（`newRawLogByMultiLine`），不是异常发生那一刻，误差为堆栈打印耗时。

### 文件采集链路（input_file，C++ core）

`input_file` 由 C++ core 实现。`Multiline` 配在输入项里（`core/input/InputFile.cpp`），`Multiline` 生效时框架在处理器链首位自动插入原生处理器 `processor_split_regex_native`（`core/pipeline/Pipeline.cpp`），真正的合并在该处理器的三态状态机里。正则是 **boost**，支持负向前瞻，所以配置面比 Go 侧宽。

```yaml
enable: true
inputs:
  - Type: input_file
    FilePaths:
      - /home/fc/logs/ehub-message-service/*.log
    Multiline:
      Mode: custom
      StartPattern: '^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}'
      UnmatchedContentTreatment: single_line
flushers:
  - Type: flusher_kafka_v2
    Brokers:
      - kafka1-in.56linked.cn:9094
    Topic: test_ehub_message
    Version: "3.4.0"
    Convert:
      Protocol: custom_single_flatten
      Encoding: json
```

`StartPattern` 同样要按真实首行格式改，取证命令：

```bash
head -50 /home/fc/logs/ehub-message-service/*.log
```

无稳定时间戳前缀时的两段式写法：

```yaml
    Multiline:
      Mode: custom
      StartPattern: '^[^\t]'
      ContinuePattern: '^(?:\s+at\s|Caused by:|\.\.\.\s\d+\s+more|Suppressed:)'
      UnmatchedContentTreatment: single_line
```

字段与约束（`core/file_server/MultilineOptions.cpp`）：

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `Multiline.Mode` | `custom` | 可选 `JSON`（按 JSON 对象边界合并） |
| `Multiline.StartPattern` | 空 | 块起始行 |
| `Multiline.ContinuePattern` | 空 | 续行，可匹配 0 到多次 |
| `Multiline.EndPattern` | 空 | 块结束行 |
| `Multiline.UnmatchedContentTreatment` | `single_line` | 可选 `discard`，丢弃无法归并的行 |

硬约束：`StartPattern` 与 `EndPattern` 都为空、只给 `ContinuePattern` 时，**整段 `Multiline` 会被忽略并告警**，至少要配 Start 或 End 之一。

### Convert.Protocol 合法值

`custom_single`、`custom_single_flatten`、`otlp_v1`、`influxdb`、`jsonline`、`raw`（`pkg/protocol/converter/converter.go`）。写错会直接返回 `unsupported protocol`，日志不投递。本 fork 的 `log_id` 在 `custom_single_flatten` 路径生成（`converter_single_log_flatten.go`）。

### 生效方式

配置文件下发到 ConfigMap（`ilogtail-user-cm` 等）后需要滚动重启采集 DaemonSet 才会重载：

```bash
kubectl -n ilogtail get cm
kubectl -n ilogtail rollout restart ds ilogtail-ds
```

验证是否合并成功：同一次异常的十几行应在 ClickVisual 里显示为一条，`content` 字段内含换行。
