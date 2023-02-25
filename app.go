package main

import (
	"context"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"log"
	"os"
	"wails-uploader/backend/dl"
)

const DefaultFileName = "main.ts"

// App struct
type App struct {
	ctx context.Context
	tm  *dl.TaskManager
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		tm: dl.NewTasksManager(5),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	go func() {
		a.tm.WatchTasks()
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.tm.CancelAllTasks()
}

func (a *App) GetDirPath() string {
	path, _ := os.Getwd()
	return path
}

func (a *App) GetFileName() string {
	return DefaultFileName
}

func (a *App) OpenDirectoryDialog(path string) string {
	str, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		CanCreateDirectories: true,
		DefaultDirectory:     path,
	})
	if err != nil {
		log.Println(err)
	}

	return str
}

func (a *App) GetTasksProgress(tasksUrls []string) []dl.TaskProgress {
	return a.tm.GetAllTasksProgress(tasksUrls)
}

func (a *App) RemoveTask(taskUrl string) error {
	return a.tm.RemoveTask(taskUrl)
}

func (a *App) PauseTask(taskUrl string) error {
	return a.tm.PauseTask(taskUrl)
}

func (a *App) StartTask(url string, filePath string) error {
	err := a.tm.AddTask(url, filePath)
	if err != nil {
		log.Println(err)
	}
	return err
}
