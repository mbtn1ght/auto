package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/chromedp/chromedp"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

type Example struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

type Task struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Complexity  int       `json:"complexity"`
	Memory      int       `json:"memory"`
	Time        int       `json:"time"`
	Examples    []Example `json:"examples"`
}

// Структура запроса к OpenAI-совместимому API
type ChatRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// Структура ответа
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func generateCodeWithFallback(task Task, apiKey string, model string, fallbackModel string, maxTokens int) (string, error) {
	solution, err := generateCode(task, apiKey, model, maxTokens)
	if err != nil && fallbackModel != "" && fallbackModel != model {
		errStr := err.Error()
		// Проверяем, что это ошибка 402 (недостаточно кредитов)
		if strings.Contains(errStr, "402") || strings.Contains(errStr, "more credits") || strings.Contains(errStr, "requires more credits") {
			log.Printf("Недостаточно кредитов для модели %s, переключаемся на запасную модель %s с большим лимитом токенов...", model, fallbackModel)
			return generateCode(task, apiKey, fallbackModel, 5000) // У запасной модели обычно больше лимит
		}
	}

	return solution, err
}

func generateCode(task Task, apiKey string, model string, maxTokens int) (string, error) {
	prompt := fmt.Sprintf("Напиши решение задачи на MinGW GNU C++ 15.2.0, отправь в ответ только код программы, без комментариев и лишних рассуждений. Ответ который ты дашь будет проходить множество тестов, так что твое решение должно быть универсальным для любых вводных данных и соответствовать требованиям по ограничениям на память и время выполнения программы. Не стесняйся долго отвечать, твой ответ должен быть максимально продуманным!\nВажно: предыдущие 5 попыток решения этой задачи НЕ ДАЛИ ВЕРНОГО РЕЗУЛЬТАТА. Тебе нужно сгенерировать ЭФФЕКТИВНОЕ РЕШЕНИЕ, которое пройдет все тесты. Задача взята с сайта acmp.ru id ниже является ее номером в списке, а name - названиею. Не пиши ```cpp  в начале и ``` в конце. Игнорируй текст похожий на 'OUTPUT.TXT1252 4 5 3 12 5 1 3 42 5 1 3 4[Лучшие попытки]' - это ошибка извлечения текста.\n\n")
	prompt += "Также ты вероятно сможешь найти решение в интренете по названию задачи и её описанию, используй это для генерации кода.\n\n"
	prompt += fmt.Sprintf("ID задачи: %d\nНазвание задачи: %s\n", task.ID, task.Name)
	prompt += fmt.Sprintf("Ограничения: память = %d КБ, время = %d мс.\n\n"+
		task.Description, task.Memory, task.Time)
	for _, ex := range task.Examples {
		prompt += fmt.Sprintf("Входные данные: %s\nВыходные данные: %s\n", ex.Input, ex.Output)
	}

	log.Println(prompt)

	reqBody := ChatRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: prompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ошибка сериализации запроса: %v", err)
	}

	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/your-repo")
	req.Header.Set("X-Title", "ACMP Auto Solver")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка HTTP запроса: %v", err)
	}
	defer resp.Body.Close()

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	// Логируем ответ для отладки
	log.Printf("HTTP статус: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		log.Printf("Ответ API: %s", string(respData))
		return "", fmt.Errorf("API вернул ошибку (статус %d): %s", resp.StatusCode, string(respData))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respData, &chatResp); err != nil {
		log.Printf("Ошибка парсинга JSON ответа: %v", err)
		log.Printf("Сырой ответ: %s", string(respData))
		return "", fmt.Errorf("ошибка парсинга ответа: %v", err)
	}

	// Проверяем наличие ошибки в ответе
	if chatResp.Error != nil {
		return "", fmt.Errorf("ошибка API: %s (тип: %s)", chatResp.Error.Message, chatResp.Error.Type)
	}

	if len(chatResp.Choices) == 0 {
		log.Printf("Пустой массив choices в ответе. Полный ответ: %s", string(respData))
		return "", fmt.Errorf("пустой ответ от ИИ")
	}

	content := chatResp.Choices[0].Message.Content
	if content == "" {
		log.Printf("Пустой content в ответе. Полный ответ: %s", string(respData))
		return "", fmt.Errorf("пустой контент в ответе ИИ")
	}

	return content, nil
}

func New(acmpLogin string, acmpPassword string, apiKey string, model string, taskID int) {
	const langSelector = `body > table > tbody > tr:nth-child(3) > td > table > tbody > tr > td:nth-child(2) > table > tbody > tr:nth-child(2) > td:nth-child(2) > form > table > tbody > tr:nth-child(1) > td:nth-child(2) > select`

	log.Println("Запуск программы...")

	data, err := ioutil.ReadFile("result.json")
	if err != nil {
		log.Fatal("Не удалось прочитать result.json:", err)
	}
	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		log.Fatal("Ошибка разбора JSON:", err)
	}

	var task Task
	found := false
	for _, t := range tasks {
		if t.ID == taskID {
			task = t
			found = true
			break
		}
	}
	if !found {
		log.Fatalf("Задача с id=%d не найдена", taskID)
	}
	log.Println("Задача найдена:", task.Name)

	log.Println("Генерация решения с помощью ИИ...")

	fallbackModel := "kwaipilot/kat-coder-pro:free"

	solution, err := generateCodeWithFallback(task, apiKey, model, fallbackModel, 16000)
	if err != nil {
		log.Fatal("Ошибка генерации кода:", err)
	}

	solution = strings.TrimSpace(solution)
	solution = strings.TrimPrefix(solution, "```cpp")
	solution = strings.TrimSuffix(solution, "```")
	solution = strings.TrimSpace(solution)
	fmt.Println(solution)
	log.Println("Решение сгенерировано")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-setuid-sandbox", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	ctxTmp, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()

	ctx, cancel := context.WithTimeout(ctxTmp, 300*time.Second)
	defer cancel()

	log.Println("Инициализация браузера...")
	err = chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
	if err != nil {
		log.Fatal("Ошибка инициализации браузера:", err)
	}

	log.Println("Запуск браузера и логин на ACMP...")

	taskURL := fmt.Sprintf("https://acmp.ru/index.asp?main=task&id_task=%d", taskID)

	loginContainer := `body > table > tbody > tr:nth-child(1) > td > table > tbody > tr:nth-child(3) > td:nth-child(4)`
	loginFormSelector := loginContainer + ` > form > nobr > b > input[type=text]:nth-child(1)`
	passwordFormSelector := loginContainer + ` > form > nobr > b > input[type=password]:nth-child(2)`
	loginButtonSelector := loginContainer + ` > form > nobr > b > input.button`

	maxRetries := 3
	var navErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Попытка навигации %d из %d...", attempt, maxRetries)
		navErr = chromedp.Run(ctx,
			chromedp.Navigate(taskURL),
			chromedp.WaitVisible(`body`, chromedp.ByQuery),
			chromedp.Sleep(3*time.Second),
		)
		if navErr == nil {
			log.Println("Навигация успешна")
			break
		}
		log.Printf("Ошибка навигации (попытка %d/%d): %v", attempt, maxRetries, navErr)
		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 5 * time.Second
			log.Printf("Ожидание %v перед следующей попыткой...", waitTime)
			chromedp.Run(ctx, chromedp.Sleep(waitTime))
		}
	}
	if navErr != nil {
		log.Fatalf("Не удалось выполнить навигацию после %d попыток: %v", maxRetries, navErr)
	}

	var loginFormExists bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`
			(function() {
				try {
					var form = document.querySelector("%s");
					return form !== null && form.offsetParent !== null;
				} catch(e) {
					return false;
				}
			})()
		`, loginFormSelector), &loginFormExists),
	)
	if err != nil {
		log.Println("Предупреждение: не удалось проверить наличие формы логина:", err)
		loginFormExists = false
	}

	if loginFormExists {
		log.Println("Форма логина найдена, выполняем вход...")
		loginCtx, loginCancel := context.WithTimeout(ctx, 30*time.Second)
		defer loginCancel()

		err = chromedp.Run(loginCtx,
			chromedp.WaitVisible(loginFormSelector, chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			log.Println("Предупреждение: форма логина не появилась в течение таймаута, возможно уже залогинены:", err)
		} else {
			// Заполняем форму и логинимся
			err = chromedp.Run(ctx,
				chromedp.SendKeys(loginFormSelector, acmpLogin),
				chromedp.SendKeys(passwordFormSelector, acmpPassword),
				chromedp.Click(loginButtonSelector),
				chromedp.Sleep(5*time.Second),
			)
			if err != nil {
				log.Fatal("Ошибка логина:", err)
			}
			log.Println("Логин выполнен")
		}
	} else {
		log.Println("Форма логина не найдена, возможно уже залогинены")
	}

	var loginText string
	err = chromedp.Run(ctx,
		chromedp.Sleep(2*time.Second),
		chromedp.Text(loginContainer, &loginText, chromedp.ByQuery),
	)
	if err != nil {
		log.Println("Предупреждение: не удалось получить текст после логина:", err)
	} else {
		log.Println("Текст контейнера логина:", loginText)
	}

	log.Println("Ожидание загрузки формы отправки решения...")

	langCtx, langCancel := context.WithTimeout(ctx, 60*time.Second)
	defer langCancel()

	err = chromedp.Run(langCtx,
		chromedp.WaitVisible(langSelector, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		log.Fatal("Селектор языка не появился, возможно проблема с загрузкой страницы или авторизацией:", err)
	}

	log.Println("Выбор языка программирования MinGW GNU C++ 15.2.0...")
	err = chromedp.Run(ctx,
		chromedp.Evaluate(fmt.Sprintf(`
				let sel = document.querySelector("%s");
				if (!sel) {
					throw new Error("Селектор языка не найден");
				}
				for (let i=0;i<sel.options.length;i++){
					if(sel.options[i].text.includes("MinGW GNU C++ 15.2.0")){
						sel.selectedIndex=i;
						sel.dispatchEvent(new Event('change'));
						break;
					}
				}
			`, langSelector), nil),
	)
	if err != nil {
		log.Fatal("Ошибка выбора языка:", err)
	}

	log.Println("Ожидание CodeMirror и вставка решения...")

	codeSelector := `div.CodeMirror`
	submitButton := `body > table > tbody > tr:nth-child(3) > td > table > tbody > tr > td:nth-child(2) > table > tbody > tr:nth-child(2) > td:nth-child(2) > form > input.button`

	err = chromedp.Run(ctx,
		chromedp.WaitVisible(codeSelector, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(fmt.Sprintf(`
				let cm = document.querySelector("%s").CodeMirror;
				cm.setValue(%q);
				cm.focus();
				cm.refresh();
				cm.getInputField().dispatchEvent(new Event('input', { bubbles: true }));
			`, codeSelector, solution), nil),
		chromedp.Sleep(1*time.Second),
		chromedp.Click(submitButton),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		log.Fatal("Ошибка вставки решения или отправки:", err)
	}

	log.Println("Решение отправлено. Программа завершена.")

	if err := MarkAsProcessed(taskID); err != nil {
		log.Printf("Предупреждение: не удалось сохранить задачу %d в список обработанных: %v", taskID, err)
	} else {
		log.Printf("Задача %d добавлена в список обработанных", taskID)
	}

	cancelCtx()
	cancelAlloc()
}
