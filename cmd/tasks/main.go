package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
)

func main() {
	config.LoadEnvironment()
	runtime, telemetryErr := telemetry.Setup(context.Background(), telemetry.ServiceConfig{ServiceName: "luna-tasks"})
	if telemetryErr != nil {
		_, _ = os.Stderr.WriteString("initialize telemetry failed\n")
		os.Exit(1)
	}
	defer func() { _ = runtime.Shutdown(context.Background()) }()
	if err := run(os.Args[1:]); err != nil {
		telemetry.Logger().Error("task administration command failed",
			slog.String("event.name", "tasks.command.failed"),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("tasks", flag.ContinueOnError)
	queue := flags.String("queue", "light", "asynq queue name")
	taskID := flags.String("task-id", "", "asynq task id")
	pageSize := flags.Int("page-size", 30, "list page size")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("usage: tasks [list-archived|run|delete] -queue <queue> [-task-id <id>]")
	}

	cfg := config.Load()
	if err := cfg.ValidateRedis(); err != nil {
		return err
	}
	inspector := asynq.NewInspector(cfg.RedisOptions().Asynq())
	defer inspector.Close()

	switch flags.Arg(0) {
	case "list-archived":
		tasks, err := inspector.ListArchivedTasks(*queue, asynq.PageSize(*pageSize))
		if err != nil {
			return err
		}
		for _, task := range tasks {
			fmt.Printf("%s\t%s\t%s\t%d\t%s\n", task.ID, task.Queue, task.Type, task.Retried, task.LastErr)
		}
		return nil
	case "run":
		if *taskID == "" {
			return fmt.Errorf("-task-id is required")
		}
		return inspector.RunTask(*queue, *taskID)
	case "delete":
		if *taskID == "" {
			return fmt.Errorf("-task-id is required")
		}
		return inspector.DeleteTask(*queue, *taskID)
	default:
		return fmt.Errorf("unknown command %q", flags.Arg(0))
	}
}
