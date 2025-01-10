package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/Conflux-Chain/go-conflux-sdk"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-util/parallel"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var flags struct {
	Url       string
	RpcOption sdk.ClientOption

	EpochFrom uint64
	NumEpochs uint64

	ParallelOption parallel.SerialOption
	ReportInterval time.Duration
}

func main() {
	cmd := cobra.Command{
		Use:   "go-test",
		Short: "QB test tool",
		Run:   test,
	}

	cmd.Flags().StringVar(&flags.Url, "url", "https://main.confluxrpc.com/DisjEM6Gno6BJP2n9FhiDH4Ev6aMfSNU8GqzGtXAZTG8zZXbKNSmcXwa5uK75pWBKmdRCJ7aNwN4R5ckKv5kxJnFc", "Fullnode RPC endpoint")
	cmd.Flags().DurationVar(&flags.RpcOption.RequestTimeout, "rpc-timeout", 3*time.Second, "Fullnode RPC timeout")
	cmd.Flags().Uint64Var(&flags.EpochFrom, "epoch-from", 0, "Epoch number to test from")
	cmd.Flags().Uint64Var(&flags.NumEpochs, "epoch-count", 30, "Number of epochs to test")
	cmd.Flags().DurationVar(&flags.ReportInterval, "report-interval", time.Second, "Interval to report progress")
	cmd.Flags().IntVar(&flags.ParallelOption.Routines, "threads", 1, "Number of threads to query RPC")

	if err := cmd.Execute(); err != nil {
		logrus.WithError(err).Fatal("Failed to execute command")
	}
}

func test(*cobra.Command, []string) {
	// create client
	client, err := sdk.NewClient(flags.Url, flags.RpcOption)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create client")
	}
	defer client.Close()

	// verify latest finalized epoch
	latestFinalizedEpoch, err := client.GetEpochNumber(types.EpochLatestFinalized)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get latest epoch number")
	}
	epochTo := flags.EpochFrom + flags.NumEpochs
	if epochTo > latestFinalizedEpoch.ToInt().Uint64() {
		logrus.WithField("finalized", latestFinalizedEpoch.ToInt()).Fatal("Not enough finalized epochs to test")
	}

	// retrieve data from RPC server
	start := time.Now()
	stat := RpcStat{
		client:         client,
		epochFrom:      flags.EpochFrom,
		lastReportTime: start,
	}
	if err = parallel.Serial(context.Background(), &stat, int(flags.NumEpochs), flags.ParallelOption); err != nil {
		logrus.WithError(err).Fatal("Failed to parallel execute RPC statistics")
	}

	data, _ := json.MarshalIndent(stat, "", "    ")
	fmt.Println(string(data))

	elapsed := time.Since(start)
	fmt.Println("Total elapsed:", elapsed)
	fmt.Println("Avg epoch latency:", time.Since(start)/time.Duration(flags.NumEpochs))
}

type EpochData struct {
	Blocks   []*types.Block
	Receipts [][]types.TransactionReceipt
	Traces   []*types.LocalizedBlockTrace
}

func QueryEpochData(client *sdk.Client, epochNumber uint64) (EpochData, error) {
	var result EpochData

	// blocks
	epoch := types.NewEpochNumberUint64(epochNumber)
	blocks, err := client.GetBlocksByEpoch(epoch)
	if err != nil {
		return EpochData{}, errors.WithMessage(err, "Failed to get blocks by epoch")
	}

	for _, blockHash := range blocks {
		// block detail
		block, err := client.GetBlockByHash(blockHash)
		if err != nil {
			return EpochData{}, errors.WithMessagef(err, "Failed to get block by hash %v", blockHash)
		}
		result.Blocks = append(result.Blocks, block)

		// traces
		blockTrace, err := client.GetBlockTraces(blockHash)
		if err != nil {
			return EpochData{}, errors.WithMessagef(err, "Failed to get block traces by block hash %v", blockHash)
		}
		result.Traces = append(result.Traces, blockTrace)
	}

	// receipts
	result.Receipts, err = client.GetEpochReceipts(*types.NewEpochOrBlockHashWithEpoch(epoch))
	if err != nil {
		return EpochData{}, errors.WithMessage(err, "Failed to get epoch receipts")
	}

	return result, nil
}

type RpcStat struct {
	client    *sdk.Client
	epochFrom uint64

	lastReportTime time.Time

	NumBlocks int
	NumTxs    int
	NumLogs   int
	NumTraces int

	NumErrors int
}

func (stat *RpcStat) ParallelDo(ctx context.Context, routine, task int) (EpochData, error) {
	return QueryEpochData(stat.client, stat.epochFrom+uint64(task))
}

func (stat *RpcStat) ParallelCollect(ctx context.Context, result *parallel.Result[EpochData]) error {
	// report progress
	if flags.ReportInterval > 0 && time.Since(stat.lastReportTime) > flags.ReportInterval {
		logrus.WithField("completed", result.Task+1).WithField("total", flags.NumEpochs).Debug("Progress update")
		stat.lastReportTime = time.Now()
	}

	if result.Err != nil {
		logrus.WithError(result.Err).WithField("epoch", stat.epochFrom+uint64(result.Task)).Warn("Failed to query epoch data")
		stat.NumErrors++
		return nil
	}

	stat.NumBlocks += len(result.Value.Blocks)
	for _, block := range result.Value.Blocks {
		stat.NumTxs += len(block.Transactions)
	}
	for _, blockReceipts := range result.Value.Receipts {
		for _, receipt := range blockReceipts {
			stat.NumLogs += len(receipt.Logs)
		}
	}
	for _, blockTraces := range result.Value.Traces {
		if blockTraces != nil {
			stat.NumTraces += len(blockTraces.TransactionTraces)
		}
	}

	return nil
}
