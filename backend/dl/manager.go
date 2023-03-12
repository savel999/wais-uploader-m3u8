package dl

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type TaskManager struct {
	sync.Mutex

	tasks      map[string]*DownloaderTask
	workersCnt int
}

func NewTasksManager(workersCnt int) *TaskManager {
	return &TaskManager{
		tasks:      make(map[string]*DownloaderTask),
		workersCnt: workersCnt,
	}
}

func (m *TaskManager) AddTask(url, path string) error {
	m.Lock()
	defer m.Unlock()

	if task, ok := m.tasks[url]; ok {
		task.Resume()

		return nil
	}

	newTask, err := NewTask(path, url)
	if err != nil {
		return err
	}

	err = m.CheckTask(newTask)
	if err != nil {
		return err
	}

	m.tasks[url] = newTask

	return nil
}

func (m *TaskManager) RemoveTask(url string) error {
	m.Lock()
	defer m.Unlock()

	if v, ok := m.tasks[url]; ok {

		if v.status == Canceled {
			v.Cancel()
			v.removeOutputFile()
		}

		v.removeTsFolder()

		delete(m.tasks, url)
	}

	return nil
}

func (m *TaskManager) PauseTask(url string) error {
	m.Lock()
	defer m.Unlock()

	if v, ok := m.tasks[url]; ok {
		v.Pause()

		return nil
	}

	return fmt.Errorf("задача с url=%s не найдена", url)
}

func (m *TaskManager) TaskProgress(url string) *TaskProgress {
	m.Lock()
	defer m.Unlock()

	if v, ok := m.tasks[url]; ok {
		progress := v.GetProgress()
		return &progress
	}

	return nil
}

func (m *TaskManager) GetAllTasksProgress(tasksUrls []string) []TaskProgress {
	m.Lock()
	defer m.Unlock()

	var result []TaskProgress

	for _, task := range m.tasks {
		for _, taskUrl := range tasksUrls {
			if taskUrl == task.url {
				result = append(result, task.GetProgress())

				break
			}
		}
	}

	return result
}

func (m *TaskManager) CheckTask(task *DownloaderTask) error {
	path := task.GetOutputFilePath()
	url := task.GetURL()

	//проверить на дубль урла и на дубль выходного файла
	for _, item := range m.tasks {
		if item.GetURL() == url {
			return fmt.Errorf("Данный урл %s уже в очереди!", url)
		}

		if item.GetOutputFilePath() == path {
			return fmt.Errorf("Данный выходной файл %s уже в очереди!", path)
		}
	}

	return nil
}

func (m *TaskManager) WatchTasks() {
	log.Println("Запуск наблюдателя по задачам")
	defer m.Unlock()

	for {
		executingTasksCnt := 0

		log.Println(m.tasks)

		for _, task := range m.tasks {
			switch task.GetStatus() {
			case Downloading, Merging:
				executingTasksCnt++
			}
		}

		if executingTasksCnt < m.workersCnt {
			m.Lock()
			for url, task := range m.tasks {
				taskStatus := task.GetStatus()
				if taskStatus == ReadyToStart {
					log.Printf("Выполняется %d задач\n", executingTasksCnt)
					log.Printf("Запуск задачи по урлу: %s\n", url)

					go task.Start()
					break
				}
			}
			m.Unlock()
		}

		//m.RemoveCompletedTasks()

		time.Sleep(1 * time.Second)
	}
}

func (m *TaskManager) CancelAllTasks() {
	log.Println("Отменяю задачи")

	for url := range m.tasks {
		err := m.RemoveTask(url)
		if err != nil {
			log.Println(fmt.Errorf("не удалось завершить задачу: %w", err))
		}
	}
}

func (m *TaskManager) RemoveCompletedTasks() {
	for url, t := range m.tasks {
		taskStatus := t.GetStatus()

		if taskStatus == Completed || taskStatus == Canceled {
			err := m.RemoveTask(url)
			if err != nil {
				log.Println(fmt.Errorf("не удалось завершить задачу: %w", err))
			}
		}
	}
}
