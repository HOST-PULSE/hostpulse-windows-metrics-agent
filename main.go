package main

import (
	"fmt"
	"os"

	// Укажи название своего модуля из go.mod, например:
	"windows-agent/services"
)

func main() {
	fmt.Println("=== Универсальный Go-агент метрик HostPulse под Windows запущен ===")

	metricsURL := os.Getenv("HOSTPULSE_METRICS_URL")
	if metricsURL == "" {
		metricsURL = "https://zedform.kz"
	}

	agentToken := os.Getenv("HOSTPULSE_TOKEN")
	if agentToken == "" {
		fmt.Println(" [⚠️ WARNING] HOSTPULSE_TOKEN не задан. Используется дефолтный токен.")
		agentToken = "default_windows_token_123"
	}

	fmt.Printf(" [INFO] Эндпоинт отправки телеметрии: %s\n", metricsURL)

	// Запускаем
	services.StartMetricsPoller(metricsURL, agentToken)
}
