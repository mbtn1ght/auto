package worker

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"
)

const processedTasksFile = "../processed_tasks.json"

var (
	processedTasks   = make(map[int]bool)
	processedTasksMu sync.RWMutex
	processedLoaded  = false
)

func LoadProcessedTasks() error {
	processedTasksMu.Lock()
	defer processedTasksMu.Unlock()

	if processedLoaded {
		return nil
	}

	processedTasks = make(map[int]bool)

	if _, err := os.Stat(processedTasksFile); os.IsNotExist(err) {
		processedLoaded = true
		return nil
	}

	data, err := ioutil.ReadFile(processedTasksFile)
	if err != nil {
		return err
	}

	var tasks []int
	if len(data) > 0 {
		if err := json.Unmarshal(data, &tasks); err != nil {
			return err
		}
	}

	for _, taskID := range tasks {
		processedTasks[taskID] = true
	}

	processedLoaded = true
	return nil
}

func IsProcessed(taskID int) bool {
	processedTasksMu.RLock()
	defer processedTasksMu.RUnlock()
	return processedTasks[taskID]
}

func MarkAsProcessed(taskID int) error {
	processedTasksMu.Lock()
	defer processedTasksMu.Unlock()

	processedTasks[taskID] = true

	var tasks []int
	for id := range processedTasks {
		tasks = append(tasks, id)
	}

	data, err := json.Marshal(tasks)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(processedTasksFile, data, 0644)
}

func GetProcessedTasks() []int {
	processedTasksMu.RLock()
	defer processedTasksMu.RUnlock()

	var tasks []int
	for id := range processedTasks {
		tasks = append(tasks, id)
	}
	return tasks
}
