@echo off
go build -o scoop-gh-proxy.exe .
if %ERRORLEVEL% neq 0 (
    echo Build failed
    exit /b %ERRORLEVEL%
)
echo Build OK: scoop-gh-proxy.exe
