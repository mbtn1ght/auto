package main

import (
	"auto-clone/worker"
	"context"
	"log"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Ошибка загрузки .env файла: %v", err)
	}
	acmpLogin := os.Getenv("ACMP_LOGIN")
	acmpPassword := os.Getenv("ACMP_PASSWORD")
	apiKey := os.Getenv("API_KEY")
	model := os.Getenv("MODEL")
	userURL := os.Getenv("USER_URL")

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.DisableGPU, chromedp.NoSandbox)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancel := context.WithTimeout(ctx, 180*time.Second) // увеличенный глобальный таймаут
	defer cancel()

	const solvedSelector = `body > table > tbody > tr:nth-child(3) > td > table > tbody > tr > td:nth-child(2) > table > tbody > tr:nth-child(2) > td:nth-child(2) > table > tbody > tr > td:nth-child(1) > p:nth-child(16)`

	solvedText, err := worker.FetchSolvedText(allocCtx, userURL, solvedSelector)
	if err != nil {
		log.Printf("Не удалось получить список решённых задач chromedp+http: %v", err)
		solvedText = ""
	}

	alreadySolved := worker.ParseSolvedTasks(solvedText)
	log.Println(alreadySolved)

	loadDone := make(chan error, 1)
	go func() {
		loadDone <- worker.LoadProcessedTasks()
	}()

	select {
	case err := <-loadDone:
		if err != nil {
			log.Printf("Предупреждение: не удалось загрузить список обработанных задач: %v", err)
		} else {
			log.Println("Список обработанных задач загружен.")
		}
	case <-time.After(5 * time.Second):
		log.Printf("Предупреждение: загрузка processed tasks заняла больше 5s и была прервана, продолжение работы.")
	}

	processedTasks := worker.GetProcessedTasks()
	log.Println("Уже обработано задач:", processedTasks)

	for taskID := 1; taskID <= 1000; taskID++ {
		skip := false
		for _, id := range alreadySolved {
			if id == taskID {
				skip = true
				break
			}
		}
		if skip {
			log.Println("Задача", taskID, "уже решена, пропускаем.")
			continue
		}

		if worker.IsProcessed(taskID) {
			log.Println("Задача", taskID, "уже обработана, пропускаем.")
			continue
		}

		log.Println("Запуск worker для задачи", taskID)
		worker.New(acmpLogin, acmpPassword, apiKey, model, taskID)
	}

	log.Println("Все задачи обработаны.")
	removePaths := []string{"./processed_tasks.json", "../processed_tasks.json"}
	removedAny := false
	for _, p := range removePaths {
		if err := os.Remove(p); err != nil {
			if !os.IsNotExist(err) {
				log.Printf("Не удалось удалить файл %s: %v", p, err)
			} else {
				log.Printf("Файл %s не найден (ok).", p)
			}
		} else {
			log.Printf("Файл %s успешно удалён.", p)
			removedAny = true
		}
	}
	if !removedAny {
		log.Println("Файлы processed_tasks.json не были найдены в проверенных путях; продолжение работы.")
	}

	log.Println("Обработка нерешённых задач...")
}
