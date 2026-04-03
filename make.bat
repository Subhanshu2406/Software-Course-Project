@echo off
REM Windows batch file wrapper for Makefile targets

if "%1"=="" (
    echo.
    echo Available targets:
    echo   make.bat build
    echo   make.bat up
    echo   make.bat down
    echo   make.bat clean
    echo   make.bat seed
    echo   make.bat token
    echo   make.bat open
    echo   make.bat open-grafana
    echo   make.bat open-prometheus
    echo.
    exit /b 0
)

powershell -ExecutionPolicy Bypass -File ".\make.ps1" -Command "%1"
