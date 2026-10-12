# Проверка: работает ли воркер Yue Studio на Windows без WSL. Описание и разбор итогов — README.md рядом.
#   powershell -ExecutionPolicy Bypass -File tools\windows-check\check-windows.ps1 [-Root C:\yue-check] [-SkipCpp]
# Всё ставится в -Root (Python, пакеты, модели, данные); система не меняется, папку можно удалить целиком.
param(
  [string]$Root = "C:\yue-check",
  [switch]$SkipCpp
)
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue" # с полосой прогресса Invoke-WebRequest качает в разы медленнее

$Repo = (Resolve-Path "$PSScriptRoot\..\..").Path
$UvVersion = "0.12.19" # как на рабочей GPU-машине
$UvSha256 = "6dbb02d79e419522f1c500f0adb1cddcff0cda7d59b0d66ea7f5e3b4a1b2f5f0"
$YueCommit = "72272f907522dcca2e97c848d6c8f0d343999183" # пакет yue2 (yue2_infer 0.1.6), как на GPU-машине
$CppRepo = "https://github.com/ServeurpersoCom/yue2.cpp.git"
$CppCommit = "c89362911b3d1038a3a14e2e560f6850f151ba70" # = CPP_COMMIT в worker/engine_cpp.py
$TorchIndex = "https://download.pytorch.org/whl/cu128"

New-Item -ItemType Directory -Force -Path $Root | Out-Null
$Report = Join-Path $Root "report.txt"
Start-Transcript -Path $Report -Force | Out-Null
$results = [ordered]@{}

function Step($name, [scriptblock]$body) {
  Write-Host "`n===== $name ====="
  $t0 = Get-Date
  try {
    $note = & $body
    $results[$name] = "OK   $note"
  } catch {
    $results[$name] = "FAIL $($_.Exception.Message)"
    Write-Host "!! $($_.Exception.Message)"
  }
  Write-Host ("----- {0}: {1:n0} с" -f $name, ((Get-Date) - $t0).TotalSeconds)
}

# вывод — через Out-Host: так он виден и попадает в report.txt, а не в итог шага;
# stderr тоже в отчёт, а Continue — чтобы строка в stderr (прогресс uv) не считалась ошибкой
function Run($exe, [string[]]$argv) {
  $ErrorActionPreference = "Continue"
  & $exe @argv 2>&1 | Out-Host
  if ($LASTEXITCODE -ne 0) { throw "$exe завершился с кодом $LASTEXITCODE" }
}

$env:UV_CACHE_DIR = "$Root\uv-cache"
$env:UV_PYTHON_INSTALL_DIR = "$Root\python"
$env:HF_HOME = "$Root\hf-cache"
$env:YUE_DATA_DIR = "$Root\data"
$env:PYTHONUTF8 = "1"
$uv = "$Root\uv\uv.exe"
$py = "$Root\venv\Scripts\python.exe"

Step "1 система" {
  Write-Host "Windows: $([Environment]::OSVersion.VersionString)"
  $free = (Get-PSDrive -Name $Root.Substring(0, 1)).Free / 1GB
  Write-Host ("свободно на диске: {0:n0} ГБ" -f $free)
  Run "nvidia-smi" @("--query-gpu=name,memory.total,driver_version", "--format=csv")
  foreach ($t in "git", "cmake", "ninja", "nvcc") {
    $c = Get-Command $t -ErrorAction SilentlyContinue
    Write-Host ("{0,-6} {1}" -f $t, $(if ($c) { $c.Source } else { "нет" }))
  }
  if ($free -lt 25) { throw ("мало места: {0:n0} ГБ, нужно ~25" -f $free) }
}

Step "2 uv и Python 3.12" {
  if (-not (Test-Path $uv)) {
    $zip = "$Root\uv.zip"
    Invoke-WebRequest "https://github.com/astral-sh/uv/releases/download/$UvVersion/uv-x86_64-pc-windows-msvc.zip" -OutFile $zip
    $got = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
    if ($got -ne $UvSha256) { throw "sha256 uv не сошлась: $got" }
    Expand-Archive $zip -DestinationPath "$Root\uv" -Force
  }
  Run $uv @("python", "install", "3.12")
  if (-not (Test-Path $py)) { Run $uv @("venv", "$Root\venv", "--python", "3.12") }
  Run $py @("-V")
}

Step "3 пакеты воркера" {
  # как install.sh на Linux, плюс сам пакет yue2: в requirements.txt его нет
  Run $uv @("pip", "install", "--python", $py, "-r", "$Repo\worker\requirements.txt",
    "yue2-infer @ https://github.com/multimodal-art-projection/YuE/archive/$YueCommit.zip",
    "--index-url", $TorchIndex, "--extra-index-url", "https://pypi.org/simple")
  Run $py @("-c", "import torch, torchaudio, yue2, demucs, lameenc; print('torch', torch.__version__, 'cuda', torch.version.cuda, 'gpu', torch.cuda.is_available(), torch.cuda.get_device_name(0) if torch.cuda.is_available() else '-')")
}

Step "4 тесты воркера (unittest, без GPU)" {
  Push-Location "$Repo\worker"
  $ErrorActionPreference = "Continue"
  try {
    & $py -m unittest discover -p "test_*.py" 2>&1 | ForEach-Object { "$_" } | Tee-Object -FilePath "$Root\unittest.txt" |
      Select-Object -Last 40 | Out-Host
    $code = $LASTEXITCODE
  } finally { Pop-Location }
  if ($code -ne 0) { throw "есть упавшие тесты — полный вывод в $Root\unittest.txt" }
}

Step "5 генерация YuE2 через воркер (черновик ~20 с)" {
  $log = "$Root\worker.log"
  $w = Start-Process -FilePath $py -ArgumentList "-m", "uvicorn", "yue_worker:app", "--host", "127.0.0.1", "--port", "8091" `
    -WorkingDirectory "$Repo\worker" -RedirectStandardOutput $log -RedirectStandardError "$Root\worker.err.log" -PassThru -NoNewWindow
  try {
    $base = "http://127.0.0.1:8091"
    $up = $false
    for ($i = 0; $i -lt 60 -and -not $up; $i++) {
      Start-Sleep 2
      try { Invoke-RestMethod "$base/health" | Out-Null; $up = $true } catch { if ($w.HasExited) { break } }
    }
    if (-not $up) { throw "воркер не поднялся — см. $Root\worker.err.log" }
    $body = @{ title = "windows-check"; style = "English, warm piano pop, expressive female voice, 88 BPM";
      lyrics = "[Verse]`nNeon fades along the lane`nFootsteps keep the time of rain"; draft = $true; seed = 831001 } | ConvertTo-Json
    $job = Invoke-RestMethod "$base/jobs" -Method Post -ContentType "application/json; charset=utf-8" -Body ([Text.Encoding]::UTF8.GetBytes($body))
    Write-Host "задача #$($job.id); первый раз модель (~10 ГБ) качается с Hugging Face"
    $deadline = (Get-Date).AddMinutes(90)
    do {
      Start-Sleep 15
      $j = Invoke-RestMethod "$base/jobs/$($job.id)"
      Write-Host ("{0:HH:mm:ss} {1}" -f (Get-Date), $j.status)
    } while ($j.status -in "queued", "running" -and (Get-Date) -lt $deadline)
    if ($j.status -ne "done") { throw "генерация: $($j.status) $($j.error)" }
    "трек $($j.duration_sec) с, файлы в $Root\data"
  } finally {
    if (-not $w.HasExited) { Stop-Process -Id $w.Id -Force }
  }
}

if (-not $SkipCpp) {
  Step "6 сборка yue2.cpp (buildcuda.cmd)" {
    $vcvars = "C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
    $miss = @()
    foreach ($t in "git", "cmake", "ninja", "nvcc") { if (-not (Get-Command $t -ErrorAction SilentlyContinue)) { $miss += $t } }
    if (-not (Test-Path $vcvars)) { $miss += "Visual Studio 18 Build Tools" }
    if ($miss) { throw "пропущено: нет $($miss -join ', ') (buildcuda.cmd ждёт именно путь $vcvars)" }
    $src = "$Root\yue2cpp-src"
    if (-not (Test-Path $src)) { Run "git" @("clone", "--quiet", $CppRepo, $src) }
    Run "git" @("-C", $src, "checkout", "--quiet", $CppCommit)
    Run "git" @("-C", $src, "submodule", "update", "--init", "--recursive", "--quiet")
    Push-Location $src
    try { Run "cmd" @("/c", "buildcuda.cmd") } finally { Pop-Location }
    $exe = "$src\build\Release\yue-server.exe"
    if (-not (Test-Path $exe)) { throw "сборка прошла, но $exe нет" }
    Run $exe @("--help")
    "собран $exe"
  }
}

Write-Host "`n===== ИТОГ ====="
$results.GetEnumerator() | ForEach-Object { Write-Host ("{0,-50} {1}" -f $_.Key, $_.Value) }
Stop-Transcript | Out-Null
Write-Host "отчёт: $Report"
