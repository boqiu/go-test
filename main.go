package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	rpc "github.com/openweb3/go-rpc-provider"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("fullnode URL not specified")
		return
	}

	urls := strings.Split(os.Args[1], ",")

	for _, v := range urls {
		go test(v)
	}

	select {}
}

func test(url string) {
	client, err := rpc.Dial(url)
	if err != nil {
		fmt.Println("Failed to connect to fullnode:", os.Args[1])
		return
	}
	defer client.Close()

	fmt.Println("Succeeded to connect to:", os.Args[1])

	for {
		epoch, err := getEpochNumber(client)
		if err != nil {
			fmt.Println("Failed to get epoch number:", err)
		} else if len(epoch) == 0 {
			fmt.Println("Invalid epoch returned")
			break
		} else {
			fmt.Println("epoch number:", epoch)
		}

		if len(os.Args) >= 3 {
			interval, _ := strconv.Atoi(os.Args[2])
			time.Sleep(time.Millisecond * time.Duration(interval))
		}
	}
}

func getEpochNumber(client *rpc.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var result string
	if err := client.CallContext(ctx, &result, "cfx_epochNumber", "latest_mined"); err != nil {
		return "", err
	}

	return result, nil
}
