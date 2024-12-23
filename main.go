package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"math/big"

	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
)

type SendCheck struct {
	Address      string
	ID           string
	Send         string
	Recv         string
	SendFee      string
	VmSend       string
	VmRecv       string
	StartBalance string
	EndBalance   string
	Result       string
}

type Config struct {
	DB          string            `json:"db"`
	Address     map[string]string `json:"address"`
	SkipVM      bool              `json:"skip_vm"`
	StartHeight int64             `json:"-"`
	EndHeight   int64             `json:"-"`
}

var (
	registry = prometheus.NewRegistry()

	balanceCheckResult = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "filecoin_balance_check_result",
			Help: "Balance check result for Filecoin addresses",
		},
		[]string{"address"},
	)
)

func init() {
	registry.MustRegister(balanceCheckResult)
}

func getBalanceAtHeight(db *sql.DB, id string, height int64) (string, error) {
	for h := height; h >= 0; h-- {
		var balance string
		err := db.QueryRow(`SELECT balance FROM actors WHERE id=$1 AND height=$2`, id, h).Scan(&balance)
		if err == nil {
			return balance, nil
		}
		if err != sql.ErrNoRows {
			return "", err
		}
	}
	return "", sql.ErrNoRows
}

func (sc *SendCheck) calculateBalance() error {
	send, ok := new(big.Int).SetString(sc.Send, 10)
	if !ok {
		return fmt.Errorf("无法解析发送金额: %s", sc.Send)
	}

	recv, ok := new(big.Int).SetString(sc.Recv, 10)
	if !ok {
		return fmt.Errorf("无法解析接收金额: %s", sc.Recv)
	}

	vmSend, ok := new(big.Int).SetString(sc.VmSend, 10)
	if !ok {
		return fmt.Errorf("无法解析 VM 发送金额: %s", sc.VmSend)
	}

	vmRecv, ok := new(big.Int).SetString(sc.VmRecv, 10)
	if !ok {
		return fmt.Errorf("无法解析 VM 接收金额: %s", sc.VmRecv)
	}

	sendFee, ok := new(big.Int).SetString(sc.SendFee, 10)
	if !ok {
		return fmt.Errorf("无法解析手续费: %s", sc.SendFee)
	}

	startBalance, ok := new(big.Int).SetString(sc.StartBalance, 10)
	if !ok {
		return fmt.Errorf("无法解析起始余额: %s", sc.StartBalance)
	}

	endBalance, ok := new(big.Int).SetString(sc.EndBalance, 10)
	if !ok {
		return fmt.Errorf("无法解析结束余额: %s", sc.EndBalance)
	}

	expected := new(big.Int)
	expected.Set(startBalance)

	totalSend := new(big.Int).Add(send, vmSend)
	expected.Sub(expected, totalSend)

	totalRecv := new(big.Int).Add(recv, vmRecv)
	expected.Add(expected, totalRecv)

	expected.Sub(expected, sendFee)

	difference := new(big.Int).Sub(expected, endBalance)
	difference.Abs(difference)
	sc.Result = difference.String()

	return nil
}

func loadConfig(filename string) (*Config, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	config := &Config{}
	if err := json.Unmarshal(file, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	if config.DB == "" {
		return nil, fmt.Errorf("数据库 URL 未配置")
	}

	if len(config.Address) == 0 {
		return nil, fmt.Errorf("地址映射未配置")
	}

	// 设置北京时区
	cst := time.FixedZone("CST", 8*3600)

	genesisTime := time.Date(2020, 8, 25, 06, 0, 0, 0, cst)

	// 计算前一天的时间范围（使用北京时间）
	now := time.Now().In(cst)
	yesterday := now.AddDate(0, 0, -1)
	startTime := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, cst)
	endTime := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, cst)

	// 计算 epoch（每个 epoch 30秒）
	config.StartHeight = int64((startTime.Sub(genesisTime).Seconds()) / 30)
	config.EndHeight = int64((endTime.Sub(genesisTime).Seconds()) / 30)

	return config, nil
}

func runCheck(config *Config) {
	db, err := sql.Open("postgres", config.DB)
	if err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
	}
	defer db.Close()

	sendChecks := make([]SendCheck, 0, len(config.Address))
	for addr, id := range config.Address {
		sc := SendCheck{
			Address: addr,
			ID:      id,
		}

		querySend := `SELECT COALESCE(SUM(value)::text, '0') FROM messages WHERE "from"=$1 AND height>=$2 AND height<=$3`
		err := db.QueryRow(querySend, addr, config.StartHeight, config.EndHeight).Scan(&sc.Send)
		if err != nil {
			log.Printf("查询地址发送 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		queryRecv := `SELECT COALESCE(SUM(value)::text, '0') FROM messages WHERE "to"=$1 AND height>=$2 AND height<=$3`
		err = db.QueryRow(queryRecv, addr, config.StartHeight, config.EndHeight).Scan(&sc.Recv)
		if err != nil {
			log.Printf("查询地址接收 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		querySendFee := `SELECT COALESCE(SUM(base_fee_burn + over_estimation_burn + miner_tip)::text, '0') FROM derived_gas_outputs WHERE "from"=$1 AND height>=$2 AND height<=$3`
		err = db.QueryRow(querySendFee, addr, config.StartHeight, config.EndHeight).Scan(&sc.SendFee)
		if err != nil {
			log.Printf("查询地址发送手续费 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		if !config.SkipVM {
			queryVmSend := `SELECT COALESCE(SUM(value)::text, '0') FROM vm_messages WHERE "from"=$1 AND height>=$2 AND height<=$3`
			err = db.QueryRow(queryVmSend, addr, config.StartHeight, config.EndHeight).Scan(&sc.VmSend)
			if err != nil {
				log.Printf("查询 vm_messages 地址发送 %s (ID: %s) 失败: %v", addr, id, err)
				continue
			}

			queryVmRecv := `SELECT COALESCE(SUM(value)::text, '0') FROM vm_messages WHERE "to"=$1 AND height>=$2 AND height<=$3`
			err = db.QueryRow(queryVmRecv, addr, config.StartHeight, config.EndHeight).Scan(&sc.VmRecv)
			if err != nil {
				log.Printf("查询 vm_messages 地址接收 %s (ID: %s) 失败: %v", addr, id, err)
				continue
			}
		} else {
			sc.VmSend = "0"
			sc.VmRecv = "0"
		}

		startBalanceStr, err := getBalanceAtHeight(db, id, config.StartHeight)
		if err != nil {
			log.Printf("查询地址开始余额 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		sc.StartBalance = startBalanceStr

		endBalanceStr, err := getBalanceAtHeight(db, id, config.EndHeight)
		if err != nil {
			log.Printf("查询地址结束余额 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		sc.EndBalance = endBalanceStr

		if err := sc.calculateBalance(); err != nil {
			log.Printf("计算余额失败 %s (ID: %s): %v", addr, id, err)
			continue
		}

		sendChecks = append(sendChecks, sc)
	}

	for _, sc := range sendChecks {
		if result, ok := new(big.Int).SetString(sc.Result, 10); ok {
			balanceCheckResult.With(prometheus.Labels{
				"address": sc.Address,
			}).Set(float64(result.Int64()))
		}
	}
	log.Printf("检查结果: %v", sendChecks)
}

func main() {
	// 设置为北京时区
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Fatalf("加载时区失败: %v", err)
	}

	c := cron.New(cron.WithLocation(location))

	// 定义检查任务函数
	checkTask := func() {
		log.Printf("开始执行余额检查任务: %s", time.Now().Format("2006-01-02 15:04:05"))

		config, err := loadConfig("config.json")
		if err != nil {
			log.Printf("加载配置失败: %v", err)
			return
		}
		log.Printf("加载配置成功: %v", config)

		runCheck(config)
		log.Printf("余额检查任务完成: %s", time.Now().Format("2006-01-02 15:04:05"))
	}

	// 程序启动时立即执行一次
	checkTask()

	// 添加定时任务
	_, err = c.AddFunc("0 2 * * *", checkTask)
	if err != nil {
		log.Fatalf("添加定时任务失败: %v", err)
	}

	c.Start()

	// 使用自定义 Registry 创建 Handler
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	go func() {
		if err := http.ListenAndServe(":2112", nil); err != nil {
			log.Fatalf("启动 metrics 服务器失败: %v", err)
		}
	}()

	log.Printf("服务已启动，将在每天凌晨2点执行检查任务，metrics 可在 :2112/metrics 访问")

	select {}
}
