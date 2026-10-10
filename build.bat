@echo off
setlocal enabledelayedexpansion
echo Building Go application...

set GOOS=windows
set EXE_NAME=scoop-gh.exe

:: Compute version from git tags: patch of the latest tag + commits since it.
:: Keep this file ASCII-only: cmd parses batch files in the ANSI codepage,
:: and UTF-8 Chinese comments get mangled into broken line parsing.
for /f "delims=" %%v in ('powershell -NoProfile -Command "$t = git describe --tags --abbrev=0 2>$null; if(-not $t){$t='v0.0.0'}; $p=$t.Substring(1).Split('.'); $a = if($t -eq 'v0.0.0'){[int](git rev-list --count HEAD)}else{[int](git rev-list --count ($t+'..HEAD'))}; Write-Output ('' + [int]$p[0] + '.' + [int]$p[1] + '.' + ([int]$p[2] + $a))"') do set VERSION=%%v

:: --version / -v only prints the target version, no build
if "%1"=="--version" (
    echo Target version: %VERSION%
    exit /b 0
)
if "%1"=="-v" (
    echo Target version: %VERSION%
    exit /b 0
)

:: Build time (pure CMD, no PowerShell needed)
for /f "tokens=2 delims==" %%i in ('wmic os get localdatetime /value') do set DT=%%i
set BUILD_TIME=%DT:~0,4%-%DT:~4,2%-%DT:~6,2%_%DT:~8,2%:%DT:~10,2%:%DT:~12,2%

:: Build for each CPU architecture into its own subdirectory.
:: All dependencies are pure Go, so cross compiling with CGO disabled is enough.
for %%a in (amd64 arm64) do (
    echo Building windows/%%a ...
    set GOARCH=%%a
    set CGO_ENABLED=0
    go build -ldflags="-s -w -X github.com/jqk/scoop-gh-proxy/internal/cli.version=%VERSION% -X github.com/jqk/scoop-gh-proxy/internal/cli.buildTime=%BUILD_TIME%" -o dist\%%a\%EXE_NAME% . || goto :fail

    if exist README.md (
        copy /y README.md dist\%%a\README.md >nul
    )
)

echo Build completed!
echo Version: %VERSION%
echo Build Time: %BUILD_TIME%
echo Output: dist\amd64\%EXE_NAME%
echo Output: dist\arm64\%EXE_NAME%
exit /b 0

:fail
echo Build failed!
pause
exit /b 1
