@echo off
setlocal

echo ============================================
echo  Buildermark Local - Windows Build
echo ============================================
echo.

:: Step 1 — Install rsrc tool (generates .syso from manifest)
echo [1/4] Installing rsrc tool...
go install github.com/akavel/rsrc@latest
if %errorlevel% neq 0 (
    echo ERROR: Failed to install rsrc. Make sure Go is in your PATH.
    exit /b 1
)

:: Step 2 — Generate Windows resource file from manifest
echo [2/4] Generating resource file from manifest...
rsrc -manifest buildermark.manifest -o rsrc.syso
if %errorlevel% neq 0 (
    echo ERROR: Failed to generate resource file.
    exit /b 1
)

:: Step 3 — Download Go dependencies
echo [3/4] Downloading dependencies...
go mod tidy
if %errorlevel% neq 0 (
    echo ERROR: Failed to resolve dependencies. Is GCC (TDM-GCC or MinGW-w64) installed?
    exit /b 1
)

:: Step 4 — Build the tray application
:: Pass a version number as the first argument: build.bat 1.2.0
set VERSION=%~1
if "%VERSION%"=="" set VERSION=dev
echo [4/4] Building buildermark-local.exe (version %VERSION%)...
go build -ldflags="-H windowsgui -X main.version=%VERSION%" -o buildermark-local.exe .
if %errorlevel% neq 0 (
    echo ERROR: Build failed.
    exit /b 1
)

echo.
echo ============================================
echo  Build successful!
echo ============================================
echo.
echo Output: buildermark-local.exe
echo.
echo Next steps:
echo   1. Build the server binary:
echo      cd ..\..\local\server
echo      go build -o buildermark-server.exe ./cmd/buildermark
echo.
echo   2. Place both .exe files in the same directory
echo   3. Run buildermark-local.exe
echo.

endlocal
