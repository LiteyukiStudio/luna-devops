package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/LiteyukiStudio/devops/internal/transferjob"
)

const controlRecordPrefix = "LUNA_VOLUME_TRANSFER_RESULT "

type controlRecord struct {
	Result    transferjob.Result `json:"result"`
	ErrorCode string             `json:"errorCode,omitempty"`
}

func main() { os.Exit(run(os.Args[1:])) }

func run(arguments []string) int {
	if len(arguments) == 0 || len(arguments) == 1 && arguments[0] == "serve" {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		return 0
	}
	if len(arguments) != 1 || arguments[0] != "import" && arguments[0] != "export" {
		writeControlRecord(controlRecord{ErrorCode: transferjob.CodeFormatUnsupported})
		return 2
	}
	config, err := transferjob.ConfigFromEnv()
	if err != nil || config.Direction != arguments[0] {
		code := transferjob.CodeJobFailed
		if err != nil {
			code = transferjob.ErrorCode(err)
		}
		writeControlRecord(controlRecord{ErrorCode: code})
		return 2
	}
	ctx, err := transferjob.ContextWithRemoteTrace(context.Background(), config.Traceparent, config.Tracestate)
	if err != nil {
		writeControlRecord(controlRecord{ErrorCode: transferjob.ErrorCode(err)})
		return 2
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	runner, err := transferjob.NewRunner(config)
	if err != nil {
		writeControlRecord(controlRecord{ErrorCode: transferjob.ErrorCode(err)})
		return 2
	}
	var source *os.File
	var destination *os.File
	if config.Direction == transferjob.DirectionImport {
		source = os.Stdin
	} else {
		destination = os.Stdout
	}
	result, err := runner.RunStream(ctx, source, destination)
	if err != nil {
		writeControlRecord(controlRecord{ErrorCode: transferjob.ErrorCode(err)})
		return 1
	}
	writeControlRecord(controlRecord{Result: result})
	return 0
}

func writeControlRecord(record controlRecord) {
	payload, err := json.Marshal(record)
	if err != nil {
		payload = []byte(`{"errorCode":"volume_transfer.job_failed"}`)
	}
	_, _ = fmt.Fprintf(os.Stderr, "%s%s\n", controlRecordPrefix, payload)
}
