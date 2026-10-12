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

:: Commit date of the version's code (HEAD), shown in the program banner.
:: %%cs = committer date in YYYY-MM-DD (the %% is batch escaping for %)
for /f "delims=" %%d in ('git log -1 --format^=%%cs') do set COMMIT_DATE=%%d

:: Build for each CPU architecture into its own subdirectory.
:: All dependencies are pure Go, so cross compiling with CGO disabled is enough.
for %%a in (amd64 arm64) do (
    echo Building windows/%%a ...
    set GOARCH=%%a
    set CGO_ENABLED=0
    go build -ldflags="-s -w -X github.com/jqk/scoop-gh-proxy/internal/cli.version=%VERSION% -X github.com/jqk/scoop-gh-proxy/internal/cli.commitDate=%COMMIT_DATE%" -o dist\%%a\%EXE_NAME% . || goto :fail

    if exist README.md (
        copy /y README.md dist\%%a\README.md >nul
    )
)

echo Build completed!
echo Version: %VERSION%
echo Commit Date: %COMMIT_DATE%
echo Output: dist\amd64\%EXE_NAME%
echo Output: dist\arm64\%EXE_NAME%
exit /b 0

:fail
echo Build failed!
pause
exit /b 1
