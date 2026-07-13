package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/hibiken/asynq"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
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
