package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx := context.Background()
	cfg, configErr := loadTasksConfig()
	runtime, telemetryErr := telemetry.Setup(ctx, telemetry.ServiceConfig{
		ServiceName:        "luna-tasks",
		Endpoint:           cfg.Telemetry.Endpoint,
		Headers:            cfg.Telemetry.Headers,
		ResourceAttributes: cfg.Telemetry.ResourceAttributes,
		LogFormat:          cfg.Telemetry.LogFormat,
		LogColor:           cfg.Telemetry.LogColor,
		LogLevel:           cfg.Telemetry.LogLevel,
		NoColor:            cfg.Telemetry.NoColor,
	})
	if telemetryErr != nil {
		telemetry.LogError(ctx, "Task administration startup failed", "tasks.startup.failed", "tasks.startup",
			"telemetry.initialization.failed",
			telemetry.WrapError("telemetry.initialization.failed", "verify the OTEL exporter configuration", "initialize telemetry", telemetryErr))
		return 1
	}
	defer func() { _ = runtime.Shutdown(context.Background()) }()
	if configErr != nil {
		telemetry.LogError(ctx, "Task administration startup failed", "tasks.startup.failed", "tasks.startup",
			"config.invalid", telemetry.WrapError("config.invalid", "verify task administration environment variables", "load task configuration", configErr))
		return 1
	}
	if err := run(os.Args[1:], os.Stdout, cfg); err != nil {
		telemetry.LogError(ctx, "Task administration command failed", "tasks.command.failed", "tasks.command",
			"tasks.command.failed", err)
		return 1
	}
	return 0
}

func run(args []string, output io.Writer, cfg tasksConfig) error {
	flags := flag.NewFlagSet("tasks", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	queue := flags.String("queue", "light", "asynq queue name")
	taskID := flags.String("task-id", "", "asynq task id")
	pageSize := flags.Int("page-size", 30, "list page size")
	if err := flags.Parse(args); err != nil {
		return telemetry.WrapError("config.invalid", "run tasks with a supported command and flags", "parse task administration command", err)
	}
	if flags.NArg() == 0 {
		return telemetry.WrapError("config.invalid", "use list-archived, run, or delete", "validate task administration command",
			fmt.Errorf("usage: tasks [list-archived|run|delete] -queue <queue> [-task-id <id>]"))
	}

	inspector := asynq.NewInspector(cfg.Redis.Asynq())
	defer inspector.Close()

	switch flags.Arg(0) {
	case "list-archived":
		tasks, err := inspector.ListArchivedTasks(*queue, asynq.PageSize(*pageSize))
		if err != nil {
			return telemetry.WrapError("dependency.redis.unavailable", "start Redis or verify REDIS_ADDR", "list archived tasks", err)
		}
		for _, task := range tasks {
			_, _ = fmt.Fprintf(output, "%s\t%s\t%s\t%d\t%s\n", task.ID, task.Queue, task.Type, task.Retried, task.LastErr)
		}
		return nil
	case "run":
		if *taskID == "" {
			return telemetry.WrapError("config.invalid", "provide -task-id", "validate run task command", fmt.Errorf("-task-id is required"))
		}
		if err := inspector.RunTask(*queue, *taskID); err != nil {
			return telemetry.WrapError("dependency.redis.unavailable", "start Redis or verify REDIS_ADDR", "run archived task", err)
		}
		return nil
	case "delete":
		if *taskID == "" {
			return telemetry.WrapError("config.invalid", "provide -task-id", "validate delete task command", fmt.Errorf("-task-id is required"))
		}
		if err := inspector.DeleteTask(*queue, *taskID); err != nil {
			return telemetry.WrapError("dependency.redis.unavailable", "start Redis or verify REDIS_ADDR", "delete archived task", err)
		}
		return nil
	default:
		return telemetry.WrapError("config.invalid", "use list-archived, run, or delete", "validate task administration command",
			fmt.Errorf("unknown command %q", flags.Arg(0)))
	}
}
