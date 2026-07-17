@echo off
cd /d "%~dp0"

where docker >nul 2>nul
if errorlevel 1 (
  echo Docker was not found. Install and start Docker Desktop first.
  pause
  exit /b 1
)

if not exist ".env" (
  echo Missing .env. Copy .env.example to .env and set both passwords first.
  pause
  exit /b 1
)

echo Starting CDK system...
echo User page: https://localhost/
echo Admin page: https://localhost/admin
docker compose up --build
pause
