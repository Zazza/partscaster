# Готовая сборка yue2.cpp для Windows (движок генерации): dist/yue2cpp-windows-x86_64/
# с yue-server.exe и libggml*.dll; без неё пакет Windows не собирается (tools/worker-tar.sh). Требует: Visual Studio Build Tools (C++), CUDA Toolkit 12.8 (nvcc), cmake, ninja.
# Библиотеки CUDA (cudart, cublas) пользователю не нужны: exe найдёт их в venv воркера — torch cu128 вшивает их
# в torch/lib, а worker/engine_cpp.py добавляет этот каталог в PATH. Файл с BOM: PowerShell 5.1 без него читает
# UTF-8 как ANSI и падает на русских комментариях.
# usage: powershell -ExecutionPolicy Bypass -File tools\yue2cpp-build.ps1   (архитектуры — env YUE2CPP_ARCHS)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot          # tools\.. — корень репозитория
Set-Location $root

$Commit = 'c893629'                               # = worker/engine_cpp.py CPP_COMMIT
$Repo = 'https://github.com/ServeurpersoCom/yue2.cpp.git'
$Archs = if ($env:YUE2CPP_ARCHS) { $env:YUE2CPP_ARCHS } else { '75;80;86;89;90;120' }
$Src = 'dist/yue2cpp-src'
$Out = 'dist/yue2cpp-windows-x86_64'

foreach ($t in 'nvcc', 'cmake', 'ninja', 'git') {
  if (-not (Get-Command $t -ErrorAction SilentlyContinue)) { throw "нет $t в PATH — сборка yue2.cpp невозможна" }
}

# vcvars64: сначала vswhere (любая версия VS с C++), затем жёсткие пути (как в buildcuda.cmd — VS 18)
$vcvars = $null
$vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
if (Test-Path $vswhere) {
  $inst = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
  if ($inst) { $c = Join-Path $inst 'VC\Auxiliary\Build\vcvars64.bat'; if (Test-Path $c) { $vcvars = $c } }
}
if (-not $vcvars) {
  foreach ($p in @(
      "${env:ProgramFiles(x86)}\Microsoft Visual Studio\18\BuildTools\VC\Auxiliary\Build\vcvars64.bat",
      "${env:ProgramFiles(x86)}\Microsoft Visual Studio\17\BuildTools\VC\Auxiliary\Build\vcvars64.bat")) {
    if (Test-Path $p) { $vcvars = $p; break }
  }
}
if (-not $vcvars) { throw 'не найден vcvars64.bat (Visual Studio Build Tools с компонентом C++)' }
Write-Host "vcvars: $vcvars"

if (-not (Test-Path "$Src/.git")) { git clone --quiet --recurse-submodules $Repo $Src }
git -C $Src fetch --quiet origin
git -C $Src checkout --quiet $Commit
git -C $Src submodule update --quiet --init --recursive

# один cmd: vcvars настраивает среду (cl, nvcc виден и так), сборка целью yue-server
$build = "$Src/build-rel"
& cmd /d /c "call `"$vcvars`" >/nul && cmake -S `"$Src`" -B `"$build`" -G `"Ninja Multi-Config`"" `
  "-DGGML_CUDA=ON" "-DGGML_NATIVE=OFF" `
  "-DCMAKE_BUILD_TYPE=Release" "-DCMAKE_CUDA_ARCHITECTURES=$Archs"
if ($LASTEXITCODE -ne 0) { throw "cmake configure: код $LASTEXITCODE" }
& cmd /d /c "call `"$vcvars`" >/nul && cmake --build `"$build`" --config Release -j $env:NUMBER_OF_PROCESSORS --target yue-server"
if ($LASTEXITCODE -ne 0) { throw "cmake build: код $LASTEXITCODE" }

if (Test-Path $Out) { Remove-Item -Recurse -Force $Out }
New-Item -ItemType Directory -Force -Path $Out | Out-Null
$rel = "$build/Release"
if (-not (Test-Path "$rel/yue-server.exe")) { $rel = $build }   # едино-конфигурная сборка кладёт рядом
Copy-Item "$rel/yue-server.exe" $Out
Copy-Item "$rel\*.dll" $Out        # ggml*.dll — рядом с exe (wildcard внутри кавычек: снаружи PS 5.1 его не склеит)
if (-not (Test-Path "$Out/ggml-cuda.dll")) { Write-Host '!! ggml-cuda.dll не найден — проверьте сборку' }
$Commit | Out-File -Encoding ascii "$Out/COMMIT"
Write-Host $Out
