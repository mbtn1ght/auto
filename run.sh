#!/bin/bash

while true; do
    echo "Запуск программы..."
    go run main/main.go   # или: go run main.go
    echo "Программа завершилась. Перезапуск через 5 секунд..."
    sleep 5
done
