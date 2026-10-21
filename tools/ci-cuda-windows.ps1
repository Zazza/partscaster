# CUDA Toolkit 12.8 на Windows-раннере GitHub Actions — для сборки yue2.cpp (tools/yue2cpp-build.ps1).
# choco ставит пакет и прописывает CUDA_PATH на уровне машины, но следующие шаги job этого не видят:
# каталог bin и CUDA_PATH передаются через GITHUB_PATH / GITHUB_ENV. Ninja — если на образе его нет.
# Версия пакета — точное имя из choco (12.8.1.572; «12.8.1.1.9» не существует).
$ErrorActionPreference = 'Stop'
choco install -y --no-progress cuda --version=12.8.1.572
if ($LASTEXITCODE -ne 0) { throw "choco install cuda: код $LASTEXITCODE" }
$cuda = [Environment]::GetEnvironmentVariable('CUDA_PATH', 'Machine')
if (-not $cuda) { throw 'CUDA_PATH не выставлен установщиком CUDA' }
"$cuda\bin" | Out-File -Append -Encoding utf8 $env:GITHUB_PATH
"CUDA_PATH=$cuda" | Out-File -Append -Encoding utf8 $env:GITHUB_ENV
if (-not (Get-Command ninja -ErrorAction SilentlyContinue)) {
  choco install -y --no-progress ninja
  if ($LASTEXITCODE -ne 0) { throw "choco install ninja: код $LASTEXITCODE" }
}
Write-Host "CUDA: $cuda"
