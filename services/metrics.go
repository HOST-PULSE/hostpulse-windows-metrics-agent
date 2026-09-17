package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"syscall"
	"time"
	"unsafe"
)

// DiskReport описывает метрики конкретного логического диска (C:, D: и т.д.)
type DiskReport struct {
	Device      string  `json:"device"`
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	UsedPercent float64 `json:"used_percent"`
}

// MetricsPayload — структура для отправки на Django-бэкенд с типом агента
type MetricsPayload struct {
	ServerToken string       `json:"server_token"`
	AgentType   string       `json:"agent_type"`
	CPUUsage    float64      `json:"cpu_usage"`
	MemUsage    float64      `json:"ram_usage"`   // <-- ПРОВЕРЬ ТУТ: имя поля MemUsage
	DiskUsage   float64      `json:"disk_usage"`
	Disks       []DiskReport `json:"disks"`
}

// Подключаем системные DLL Windows
var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFree      = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetLogicalDrives = kernel32.NewProc("GetLogicalDrives")
	procGetMemory        = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetSystemTimes   = kernel32.NewProc("GetSystemTimes")
)

// StartMetricsPoller запускает бесконечный цикл сбора и отправки системных метрик Windows
func StartMetricsPoller(backendURL, token string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	fmt.Println("🚀 [INIT] Фоновый сбор системных метрик Windows запущен в пакете services...")

	client := &http.Client{Timeout: 5 * time.Second}

	var lastIdleTime, lastKernelTime, lastUserTime uint64

	// Первая отправка сразу при старте
	lastIdleTime, lastKernelTime, lastUserTime = collectAndSend(client, backendURL, token, lastIdleTime, lastKernelTime, lastUserTime)

	for range ticker.C {
		lastIdleTime, lastKernelTime, lastUserTime = collectAndSend(client, backendURL, token, lastIdleTime, lastKernelTime, lastUserTime)
	}
}

func collectAndSend(client *http.Client, url, token string, lastIdle, lastKernel, lastUser uint64) (uint64, uint64, uint64) {
	cpuUsage, nextIdle, nextKernel, nextUser := getWindowsCPU(lastIdle, lastKernel, lastUser)

	// 1. Вызываем твой сборщик дисков и сохраняем результат в переменную
	disksList := getWindowsDisks()

	// 2. Ищем диск C: в этом списке, чтобы вытащить его процент
	var cDrivePercent float64 = 0.0
	for _, disk := range disksList {
		if disk.Device == "C:" {
			cDrivePercent = disk.UsedPercent
			break
		}
	}

	// 3. Наполняем структуру, добавляя поле DiskUsage
	payload := MetricsPayload{
		ServerToken: token,
		AgentType:   "windows-metric-agent",
		CPUUsage:    cpuUsage,
		MemUsage:    getWindowsRAM(), // <-- ИСПРАВЬ ТУТ: MemUsage вместо RAMUsage
		DiskUsage:   cDrivePercent,
		Disks:       disksList,
	}

	// Обновили лог, чтобы видеть процент диска C: прямо в консоли
	fmt.Printf(" [📊 МЕТРИКИ] CPU: %.1f%% | RAM: %.1f%% | ...\n",
		payload.CPUUsage, payload.MemUsage)

	go sendMetrics(client, url, payload)

	return nextIdle, nextKernel, nextUser
}

func getWindowsRAM() float64 {
	var memoryStatus struct {
		Length               uint32
		MemoryLoad           uint32
		TotalPhys            uint64
		AvailPhys            uint64
		TotalPageFile        uint64
		AvailPageFile        uint64
		TotalVirtual         uint64
		AvailVirtual         uint64
		AvailExtendedVirtual uint64
	}
	memoryStatus.Length = uint32(unsafe.Sizeof(memoryStatus))

	r1, _, _ := procGetMemory.Call(uintptr(unsafe.Pointer(&memoryStatus)))
	if r1 == 0 {
		return 0.0
	}
	return float64(memoryStatus.MemoryLoad)
}

func getWindowsDisks() []DiskReport {
	var reports []DiskReport

	r1, _, _ := procGetLogicalDrives.Call()
	if r1 == 0 {
		return reports
	}
	bitmask := uint32(r1)

	for i := 0; i < 26; i++ {
		if (bitmask & (1 << uint(i))) != 0 {
			driveLetter := string(rune('A' + i)) + ":"

			var freeBytes, totalBytes, totalFreeBytes uint64
			drivePtr, _ := syscall.UTF16PtrFromString(driveLetter + "\\")

			r1, _, _ := procGetDiskFree.Call(
				uintptr(unsafe.Pointer(drivePtr)),
				uintptr(unsafe.Pointer(&freeBytes)),
				uintptr(unsafe.Pointer(&totalBytes)),
				uintptr(unsafe.Pointer(&totalFreeBytes)),
			)

			if r1 != 0 && totalBytes > 0 {
				usedBytes := totalBytes - freeBytes
				totalGB := float64(totalBytes) / 1024 / 1024 / 1024
				usedGB := float64(usedBytes) / 1024 / 1024 / 1024
				percent := (usedGB / totalGB) * 100

				if totalGB > 0.1 {
					reports = append(reports, DiskReport{
						Device:      driveLetter,
						TotalGB:     totalGB,
						UsedGB:      usedGB,
						UsedPercent: percent,
					})
				}
			}
		}
	}
	return reports
}

func getWindowsCPU(lastIdle, lastKernel, lastUser uint64) (float64, uint64, uint64, uint64) {
	var idleTime, kernelTime, userTime syscall.Filetime

	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if r1 == 0 {
		return 0.0, 0, 0, 0
	}

	currentIdle := (uint64(idleTime.HighDateTime) << 32) | uint64(idleTime.LowDateTime)
	currentKernel := (uint64(kernelTime.HighDateTime) << 32) | uint64(kernelTime.LowDateTime)
	currentUser := (uint64(userTime.HighDateTime) << 32) | uint64(userTime.LowDateTime)

	if lastIdle == 0 {
		return 1.5, currentIdle, currentKernel, currentUser
	}

	idleDiff := currentIdle - lastIdle
	kernelDiff := currentKernel - lastKernel
	userDiff := currentUser - lastUser

	totalSys := kernelDiff + userDiff
	if totalSys == 0 {
		return 0.0, currentIdle, currentKernel, currentUser
	}

	cpuPercent := (float64(totalSys-idleDiff) / float64(totalSys)) * 100
	if cpuPercent < 0 {
		cpuPercent = 0
	} else if cpuPercent > 100 {
		cpuPercent = 100
	}

	return cpuPercent, currentIdle, currentKernel, currentUser
}

func sendMetrics(client *http.Client, url string, payload MetricsPayload) {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// 🔥 ТАКЖЕ ДУБЛИРУЕМ ТИП В КЛАССИЧЕСКИЙ HTTP-ЗАГОЛОВОК ДЛЯ ВЬЮХИ ДЖАНГО
	req.Header.Set("X-Agent-Type", payload.AgentType)

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
