param (
    [string]$URL = "https://hostpulse.link/",
    [string]$TOKEN = "",
    [string]$PASSWORD = ""
)
# Принудительно включаем UTF-8 кодировку для отображения текста без знаков вопроса
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

# 2. Если старая служба уже существует — останавливаем и удаляем её
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Обнаружена старая версия. Переустановка..." -ForegroundColor Yellow
    Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2

    # Безопасное удаление старой службы через sc.exe
    & sc.exe delete $ServiceName | Out-Null
}

# 3. Создаем рабочую директорию, если её нет
if (!(Test-Path $TargetDir)) {
    New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
}

# 4. Скачиваем свежий скомпилированный EXE-файл агента из релизов GitHub
# Исправлено: Ссылка теперь ведет строго на скомпилированный бинарник релиза v1.0.0
$AgentDownloadUrl = "https://raw.githubusercontent.com/HOST-PULSE/hostpulse-windows-metrics-agent/releases/download/v1.0.0/windows-metric-agent.exe"
$AgentPath = "$TargetDir\hostpulse_agent.exe"

Write-Host "Скачивание свежего бинарника..." -ForegroundColor Cyan
try {
    Invoke-WebRequest -Uri $AgentDownloadUrl -OutFile $AgentPath -UseBasicParsing
} catch {
    (New-Object System.Net.WebClient).DownloadFile($AgentDownloadUrl, $AgentPath)
}

# 5. Скачиваем NSSM по жесткому абсолютному пути
$NssmPath = "$TargetDir\nssm.exe"
if (!(Test-Path $NssmPath)) {
    Write-Host "[INFO] Скачивание системных компонентов службы..." -ForegroundColor Cyan
    $NssmUrl = "https://raw.githubusercontent.com/HOST-PULSE/hostpulse-windows-metrics-agent/main/nssm.exe"
    try {
        Invoke-WebRequest -Uri $NssmUrl -OutFile $NssmPath -UseBasicParsing
    } catch {
        (New-Object System.Net.WebClient).DownloadFile($NssmUrl, $NssmPath)
    }
}

# Дополнительная проверка на физическое наличие файлов на диске перед установкой
if (!(Test-Path $NssmPath) -or !(Test-Path $AgentPath)) {
    Write-Error "Критическая ошибка: Не все компоненты были успешно скачаны на диск!"
    exit 1
}

Write-Host "Регистрация фоновой службы Windows..." -ForegroundColor Cyan

# 6. Создаем службу через NSSM по жестким путям (исправлены относительные .\ пути)
& $NssmPath install $ServiceName $AgentPath | Out-Null
& $NssmPath set $ServiceName Description "HostPulse Windows System Metrics Agent" | Out-Null
& $NssmPath set $ServiceName AppDirectory $TargetDir | Out-Null
& $NssmPath set $ServiceName Start SERVICE_AUTO_START | Out-Null

# 7. Устанавливаем переменные окружения для службы
# Исправлено: Склеиваем массив через знак новой строки, как требует реестр Windows и NSSM
$EnvPayload = @(
    "HOSTPULSE_METRICS_URL=${URL}api/agent/metrics/",
    "HOSTPULSE_TOKEN=$TOKEN",
    "HOSTPULSE_SECRET=$PASSWORD"
) -join "`n"

& $NssmPath set $ServiceName AppEnvironmentExtra $EnvPayload | Out-Null

# 8. Запускаем службу
Start-Service -Name $ServiceName

Write-Host "[УСПЕХ] Агент HostPulse успешно установлен и запущен как служба Windows!" -ForegroundColor Green
Write-Host "Метрики будут отправляться в CRM каждые 10 секунд." -ForegroundColor Green
