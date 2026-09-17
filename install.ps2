# install.ps1
param (
    [string]$URL = "https://hostpulse.link/",
    [string]$TOKEN = "",
    [string]$PASSWORD = ""
)
[console]::InputEncoding = [System.Text.Encoding]::UTF8
[console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = "Stop"

# 1. Проверяем права Администратора
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (!$isAdmin) {
    Write-Error "❌ Ошибка: Этот скрипт нужно запускать строго от имени Администратора (Run as Administrator)!"
    exit
}

$TargetDir = "C:\Program Files\HostPulse"
$ServiceName = "HostPulseWindowsAgent"

Write-Host "🐳 [HostPulse] Начинаем установку/обновление Windows-агента..." -ForegroundColor Cyan

# 2. Если старая служба уже существует — останавливаем и удаляем её (аналог docker rm -f)
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "🔄 Обнаружена старая версия. Переустановка..." -ForegroundColor Yellow
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    # Даем Windows 2 секунды на освобождение файла бинарника
    Start-Sleep -Seconds 2

    # Удаляем службу из реестра Windows
    $serviceObj = Get-WmiObject -Class Win32_Service -Filter "Name='$ServiceName'"
    if ($serviceObj) { $serviceObj.Delete() | Out-Null }
}

# 3. Создаем рабочую директорию, если её нет
if (!(Test-Path $TargetDir)) {
    New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
}

# 4. Скачиваем свежий скомпилированный EXE-файл агента
# Замени URL ниже на точный путь к твоему релизу/артефакту в GitHub или на бэкенде
$AgentDownloadUrl = "https://github.com/HOST-PULSE/hostpulse-windows-metrics-agent"
Write-Host "📥 Скачивание свежего бинарника..." -ForegroundColor Cyan
Invoke-WebRequest -Uri $AgentDownloadUrl -OutFile "$TargetDir\hostpulse_agent.exe" -UseBasicParsing

# 5. Скачиваем NSSM (Non-Sucking Service Manager) — утилиту для работы служб на чистом Go
$NssmUrl = "https://nssm.cc"
if (!(Test-Path "$TargetDir\nssm.exe")) {
    Write-Host "[INFO] Скачивание системных компонентов службы..." -ForegroundColor Cyan

    # Ссылка ведет на чистый 64-битный исполняемый файл в вашем репозитории

    $NssmUrl = "https://raw.githubusercontent.com/HOST-PULSE/hostpulse-windows-metrics-agent/main/nssm.exe"
    try {
        Invoke-WebRequest -Uri $NssmUrl -OutFile "$TargetDir\nssm.exe" -UseBasicParsing
    } catch {
        # Резервный метод скачивания (старый WebClient), если первый дал сбой
        (New-Object System.Net.WebClient).DownloadFile($NssmUrl, "$TargetDir\nssm.exe")
    }
}
Write-Host "⚙️ Регистрация фоновой службы Windows..." -ForegroundColor Cyan

# 6. Создаем службу через NSSM
cd $TargetDir
.\nssm.exe install $ServiceName "$TargetDir\hostpulse_agent.exe"
.\nssm.exe set $ServiceName Description "HostPulse Windows System Metrics Agent"
.\nssm.exe set $ServiceName Start SERVICE_AUTO_START

# 7. 🔥 УСТАНАВЛИВАЕМ ПЕРЕМЕННЫЕ ОКРУЖЕНИЯ ДЛЯ СЛУЖБЫ
# NSSM умеет прокидывать env-переменные напрямую в контекст запуска процесса!
$EnvPayload = @(
    "HOSTPULSE_METRICS_URL=${URL}api/agent/metrics/",
    "HOSTPULSE_TOKEN=$TOKEN",
    "HOSTPULSE_SECRET=$PASSWORD"
)
.\nssm.exe set $ServiceName AppEnvironmentExtra $EnvPayload

# 8. Запускаем службу
Start-Service -Name $ServiceName

Write-Host "🟢 [УСПЕХ] Агент HostPulse успешно установлен и запущен как служба Windows!" -ForegroundColor Green
Write-Host "📊 Метрики будут отправляться в CRM каждые 10 секунд." -ForegroundColor Green
