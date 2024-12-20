# send-check

一个用于检查 Filecoin 指定账户交易余额对账的工具。

## 功能特点

- 支持指定区块高度范围进行对账
- 自动计算账户的发送、接收、手续费等交易数据
- 验证账户余额变动是否准确
- 输出 JSON 格式的对账结果

## 使用方法

```bash
./send-check -end <结束区块高度> -start <起始区块高度> -url <数据库连接地址>
```

参数说明:
- `-start`: 起始区块高度
- `-end`: 结束区块高度  
- `-url`: PostgreSQL 数据库连接地址
- `-address`: 地址映射，格式为'address1:id1,address2:id2'
- `-skip-vm`: 跳过 vm_messages 表的查询

## 输出说明

程序会输出 JSON 格式的对账结果,包含以下字段:

- `Address`: 账户地址
- `ID`: 账户 ID
- `Send`: 发送金额(attoFIL)
- `Recv`: 接收金额(attoFIL) 
- `SendFee`: 发送交易产生的手续费
- `VmSend`: VM 发送金额(attoFIL)
- `VmRecv`: VM 接收金额(attoFIL)
- `StartBalance`: 起始余额
- `EndBalance`: 结束余额
- `Result`: 对账结果(0表示账目平衡)

对账计算公式:
```
Result = StartBalance - SendFee - Send - VmSend + Recv + VmRecv - EndBalance
```
当 Result = 0 时表示账目完全平衡。

## 示例
```bash
root@lily-5-prod:~/send-check# ./send-check -end 4545100 -start 4545000 -skip-vm -url 'postgres'
[
  {
    "Address": "f1khdd2v7il7lxn4zjzzrqwceh466mq5k333ktu7q",
    "ID": "f01906216",
    "Send": "25150715184380000000000",
    "Recv": "21979951475781584299238",
    "SendFee": "34801249385521",
    "StartBalance": "3331153890278300306726070",
    "EndBalance": "3327983126534900641639787",
    "Result": "0"
  },
  {
    "Address": "f1m2swr32yrlouzs7ijui3jttwgc6lxa5n5sookhi",
    "ID": "f086971",
    "Send": "0",
    "Recv": "0",
    "SendFee": "0",
    "StartBalance": "117273712634807362343707731",
    "EndBalance": "117273712634807362343707731",
    "Result": "0"
  },
  {
    "Address": "f1ys5qqiciehcml3sp764ymbbytfn3qoar5fo3iwy",
    "ID": "f047684",
    "Send": "10673644304940000000000",
    "Recv": "236639274523400000000",
    "SendFee": "466484652384156",
    "StartBalance": "79801511390836445981203596",
    "EndBalance": "79791074385339544728819440",
    "Result": "0"
  }
]
```