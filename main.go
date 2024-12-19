package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"

	"math/big"

	_ "github.com/lib/pq"
)

type SendCheck struct {
	Address      string
	ID           string
	Send         string
	Recv         string
	SendFee      string
	StartBalance string
	EndBalance   string
	Ok           bool
}

func parseAddressMapping(mapping string) map[string]string {
	result := make(map[string]string)
	pairs := strings.Split(mapping, ",")
	for _, pair := range pairs {
		kv := strings.Split(pair, ":")
		if len(kv) == 2 {
			result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return result
}

func parseBalance(balance string) (*big.Int, error) {
	n := new(big.Int)
	_, ok := n.SetString(balance, 10)
	if !ok {
		return nil, fmt.Errorf("无法解析余额: %s", balance)
	}
	return n, nil
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
	expected.Sub(expected, send)
	expected.Add(expected, recv)
	expected.Sub(expected, sendFee)

	sc.Ok = expected.Cmp(endBalance) == 0
	return nil
}

func main() {
	var address map[string]string

	addressMapping := flag.String("address", "", "地址映射，格式为'address1:id1,address2:id2'")
	dbURL := flag.String("url", "", "数据库 URL (格式: postgres://user:password@host:port/dbname?sslmode=disable)")
	startHeight := flag.Int64("start", 0, "起始高度")
	endHeight := flag.Int64("end", 0, "结束高度")

	flag.Parse()

	if *addressMapping != "" {
		address = parseAddressMapping(*addressMapping)
	} else {
		address = map[string]string{
			"f1khdd2v7il7lxn4zjzzrqwceh466mq5k333ktu7q": "f01906216",
			"f1m2swr32yrlouzs7ijui3jttwgc6lxa5n5sookhi": "f086971",
			"f1ys5qqiciehcml3sp764ymbbytfn3qoar5fo3iwy": "f047684",
		}
	}

	if *dbURL == "" {
		log.Fatal("请提供数据库 URL 参数")
	}

	db, err := sql.Open("postgres", *dbURL)
	if err != nil {
		log.Fatalf("无法连接到数据库: %v", err)
	}
	defer db.Close()

	sendChecks := make([]SendCheck, 0, len(address))
	for addr, id := range address {
		sc := SendCheck{
			Address: addr,
			ID:      id,
		}

		querySend := `SELECT COALESCE(SUM(value)::text, '0') FROM messages WHERE method=0 AND "from"=$1 AND height>=$2 AND height<=$3`
		err := db.QueryRow(querySend, addr, *startHeight, *endHeight).Scan(&sc.Send)
		if err != nil {
			log.Printf("查询地址发送 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		queryRecv := `SELECT COALESCE(SUM(value)::text, '0') FROM messages WHERE method=0 AND "to"=$1 AND height>=$2 AND height<=$3`
		err = db.QueryRow(queryRecv, addr, *startHeight, *endHeight).Scan(&sc.Recv)
		if err != nil {
			log.Printf("查询地址接收 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		querySendFee := `SELECT COALESCE(SUM(base_fee_burn + over_estimation_burn + miner_tip)::text, '0') FROM derived_gas_outputs WHERE method=0 AND "from"=$1 AND height>=$2 AND height<=$3`
		err = db.QueryRow(querySendFee, addr, *startHeight, *endHeight).Scan(&sc.SendFee)
		if err != nil {
			log.Printf("查询地址发送手续费 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}

		startBalanceStr, err := getBalanceAtHeight(db, id, *startHeight)
		if err != nil {
			log.Printf("查询地址开始余额 %s (ID: %s) 失败: %v", addr, id, err)
			continue
		}
		sc.StartBalance = startBalanceStr

		endBalanceStr, err := getBalanceAtHeight(db, id, *endHeight)
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

	fmt.Println(sendChecks)
}
