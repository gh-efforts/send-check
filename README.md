# send-check

一个自动化的 Filecoin 链上交易对账工具，支持定时任务和 Prometheus 指标监控。用来确认lily数据库中是否缺数据。

## 功能特点

- 自动计算前一天的区块高度范围进行对账
- 支持多账户批量对账
- 提供 Prometheus 指标监控
- 定时任务支持（默认每天凌晨 2 点执行）
- 可选是否包含 VM 消息交易统计

## 配置说明

需要在程序同目录下创建 `config.json` 配置文件：

```json
{
  "db": "postgresql://user:password@localhost:5432/dbname",
  "address": {
    "f1xxx...": "f01234",
    "f1yyy...": "f05678"
  },
  "skip_vm": false
}
```

配置项说明：
- `db`: PostgreSQL 数据库连接串
- `address`: Filecoin 地址与 ID 的映射关系
- `skip_vm`: 是否跳过 VM 消息统计（可选，默认 false）

## 对账结果说明

程序会输出 JSON 格式的对账结果，包含以下字段：

| 字段 | 说明 |
|------|------|
| `Address` | Filecoin 账户地址 |
| `ID` | 账户 ID |
| `Send` | 发送金额 (attoFIL) |
| `Recv` | 接收金额 (attoFIL) |
| `SendFee` | 发送交易手续费 |
| `VmSend` | VM 发送金额 (attoFIL) |
| `VmRecv` | VM 接收金额 (attoFIL) |
| `StartBalance` | 起始余额 |
| `EndBalance` | 结束余额 |
| `Result` | 对账结果 (0 表示平衡) |

### 对账计算公式

```
Result = StartBalance - SendFee - (Send + VmSend) + (Recv + VmRecv) - EndBalance
```

## Prometheus 指标

程序在 `:2112/metrics` 端点提供以下指标：

- `filecoin_balance_check_result{address="xxx"}`: 各地址的对账结果

## 数据库查询说明

程序会查询以下数据表：
- `messages`: 链上交易消息
- `vm_messages`: VM 消息（可选）
- `actors`: 账户余额信息
- `derived_gas_outputs`: 手续费信息

## 时间设置

- 程序使用北京时间（Asia/Shanghai）
- 自动计算前一天（0:00-23:59:59）的区块高度
- 定时任务在每天凌晨 2 点执行

## 运行说明

1. 准备配置文件 `config.json`
2. 运行程序：
```bash
./send-check
```

程序启动后会：
1. 立即执行一次对账检查
2. 设置每天凌晨 2 点的定时任务
3. 启动 Prometheus 指标服务器（端口 2112）