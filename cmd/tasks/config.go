package main

import sharedconfig "github.com/LiteyukiStudio/devops/internal/config"

type tasksConfig = sharedconfig.TasksConfig

func loadTasksConfig() (tasksConfig, error) {
	return sharedconfig.LoadTasks()
}
